// Copyright © 2025 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package perf

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"runtime/pprof"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	"github.com/LFDT-Paladin/paladin/perf/internal/testsuite"
	"github.com/LFDT-Paladin/paladin/perf/internal/util"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"

	dto "github.com/prometheus/client_model/go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

const (
	workerPrefix          = "worker-"
	runnerShutdownTimeout = 30 * time.Second
)

var METRICS_NAMESPACE = "pldperf"
var METRICS_SUBSYSTEM = "runner"

var totalActionsCounter = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: METRICS_NAMESPACE,
	Name:      "actions_submitted_total",
	Subsystem: METRICS_SUBSYSTEM,
})

var receivedEventsCounter = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: METRICS_NAMESPACE,
	Name:      "received_events_total",
	Subsystem: METRICS_SUBSYSTEM,
})

var incompleteEventsCounter = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: METRICS_NAMESPACE,
	Name:      "incomplete_events_total",
	Subsystem: METRICS_SUBSYSTEM,
})

var perfTestDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: METRICS_NAMESPACE,
	Subsystem: METRICS_SUBSYSTEM,
	Name:      "perf_test_duration_seconds",
	Buckets:   []float64{1.0, 2.0, 5.0, 10.0, 30.0},
}, []string{"test"})

func Init() {
	prometheus.Register(receivedEventsCounter)
	prometheus.Register(incompleteEventsCounter)
	prometheus.Register(totalActionsCounter)
	prometheus.Register(perfTestDurationHistogram)
}

func getMetricVal(collector prometheus.Collector) float64 {
	collectorChannel := make(chan prometheus.Metric, 1)
	collector.Collect(collectorChannel)
	metric := dto.Metric{}
	err := (<-collectorChannel).Write(&metric)
	if err != nil {
		log.Errorf("error writing metric: %s", err)
	}
	if metric.Counter != nil {
		return *metric.Counter.Value
	} else if metric.Gauge != nil {
		return *metric.Gauge.Value
	}
	return 0
}

type PerfRunner interface {
	Init() error
	Start() error
}

type summary struct {
	mutex        *sync.Mutex
	rampSummary  int64
	totalSummary int64
}
type perfRunner struct {
	bfr               chan int
	cfg               *conf.RunnerConfig
	nodes             []*testsuite.Node // HTTP + WS client and config per node
	ctx               context.Context
	shutdown          context.CancelFunc
	stopping          bool
	closingWebsocket  bool // Flag to indicate websocket is being closed in cleanup
	nodeKilled        bool // Flag to stop accepting new transactions after node kill
	waitingForRestart bool // Flag to indicate we're waiting for node restart

	startTime     int64
	endSendTime   int64
	endTime       int64
	startRampTime int64
	endRampTime   int64

	totalWorkers  int
	reportBuilder *util.Report
	sendTime      *util.Latency
	receiveTime   *util.Latency
	totalTime     *util.Latency
	summary       summary
	workerIDMap   sync.Map

	wsReceivers map[string]chan bool

	currentSuite testsuite.TestSuite
	nodeManager  NodeManager

	// Node kill coordination channels (one set per worker)
	pauseRequests []chan struct{} // Channels to signal each worker to pause
	pauseAcks     []chan struct{} // Channels for each worker to acknowledge they've paused
	resumeSignals []chan struct{} // Channels to signal each worker to resume

	wg sync.WaitGroup // tracks worker and event-loop goroutines for graceful shutdown
}

func New(config *conf.RunnerConfig, reportBuilder *util.Report) PerfRunner {
	if config.LogLevel != "" {
		if level, err := log.ParseLevel(config.LogLevel); err == nil {
			log.SetLevel(level)
		}
	}

	totalWorkers := config.Test.Workers

	ctx, cancel := context.WithCancel(context.Background())

	startRampTime := time.Now().Unix()
	endRampTime := time.Now().Unix() + int64(config.RampLength.Seconds())
	startTime := endRampTime
	endTime := startTime + int64(config.Length.Seconds())

	pr := &perfRunner{
		bfr:           make(chan int, totalWorkers),
		cfg:           config,
		ctx:           ctx,
		shutdown:      cancel,
		startRampTime: startRampTime,
		endRampTime:   endRampTime,
		startTime:     startTime,
		endTime:       endTime,
		reportBuilder: reportBuilder,
		sendTime:      &util.Latency{},
		receiveTime:   &util.Latency{},
		totalTime:     &util.Latency{},
		workerIDMap:   sync.Map{},
		summary: summary{
			totalSummary: 0,
			mutex:        &sync.Mutex{},
		},
		totalWorkers:  totalWorkers,
		pauseRequests: make([]chan struct{}, totalWorkers),
		pauseAcks:     make([]chan struct{}, totalWorkers),
		resumeSignals: make([]chan struct{}, totalWorkers),
	}
	// Initialize channels for each worker
	for i := 0; i < totalWorkers; i++ {
		pr.pauseRequests[i] = make(chan struct{}, 1)
		pr.pauseAcks[i] = make(chan struct{}, 1)
		pr.resumeSignals[i] = make(chan struct{}, 1)
	}
	return pr
}

func (pr *perfRunner) Init() (err error) {
	// All tests require at least one node
	if len(pr.cfg.Nodes) == 0 {
		return fmt.Errorf("at least one node must be configured")
	}

	// Create distinct HTTP and WebSocket clients for every node
	pr.nodes = make([]*testsuite.Node, 0, len(pr.cfg.Nodes))
	for i, nodeCfg := range pr.cfg.Nodes {
		httpConfig := pr.cfg.HTTPConfig
		httpConfig.URL = nodeCfg.HTTPEndpoint
		httpClient, err := pldclient.New().HTTP(pr.ctx, &httpConfig)
		if err != nil {
			return fmt.Errorf("failed to create HTTP client for node %d: %w", i, err)
		}
		wsConfig := pr.cfg.WSConfig
		wsConfig.URL = nodeCfg.WSEndpoint
		wsClient, err := pldclient.New().WebSocket(pr.ctx, &wsConfig)
		if err != nil {
			return fmt.Errorf("failed to create WebSocket client for node %d: %w", i, err)
		}
		pr.nodes = append(pr.nodes, &testsuite.Node{
			HTTPClient: httpClient,
			WSClient:   wsClient,
			Config:     nodeCfg,
		})
	}

	// Initialize node manager if node kill config is provided
	if pr.cfg.NodeKillConfig != nil {
		pr.nodeManager = NewNodeManager(pr.cfg.NodeKillConfig, pr.cfg.Nodes)
	}

	return nil
}

// GetNodes implements testsuite.Runner so suites can access node-scoped HTTP and WS clients.
func (pr *perfRunner) GetNodes() []*testsuite.Node {
	return pr.nodes
}

func (pr *perfRunner) Start() (err error) {
	log.Infof("Running test:\n%+v", pr.cfg)

	test := pr.cfg.Test

	// Create channel based dispatch for workers
	pr.wsReceivers = make(map[string]chan bool)
	for i := 0; i < pr.totalWorkers; i++ {
		prefixedWorkerID := fmt.Sprintf("%s%d", workerPrefix, i)
		pr.wsReceivers[prefixedWorkerID] = make(chan bool, test.ActionsPerLoop)
	}

	suite := testsuite.GetTestSuite(test.Name, pr.ctx, pr)
	if suite == nil {
		return fmt.Errorf("unknown test case '%s'", test.Name)
	}
	pr.currentSuite = suite

	if err = suite.Setup(); err != nil {
		return fmt.Errorf("failed to setup %s test: %w", test.Name, err)
	}
	sub, err := suite.Subscribe()
	if err != nil {
		return err
	}
	if sub != nil {
		pr.wg.Add(1)
		go pr.batchEventLoop(sub)
	}

	// Start workers
	log.Infof("Starting %d workers for case \"%s\"", test.Workers, test.Name)
	workerErrCh := make(chan error, test.Workers)
	id := 0
	for iWorker := 0; iWorker < test.Workers; iWorker++ {
		tc := suite.NewWorker(pr.startTime, id)

		delayPerWorker := pr.cfg.RampLength / time.Duration(test.Workers)

		pr.wg.Add(1)
		go func(workerID int) {
			// Delay the start of the next worker by (ramp time) / (number of workers)
			if delayPerWorker > 0 {
				time.Sleep(delayPerWorker * time.Duration(workerID))
				log.Infof("Ramping up. Starting next worker after waiting %v", delayPerWorker)
			}
			err := pr.runLoop(tc, workerID, test.ActionsPerLoop)
			if err != nil {
				log.Errorf("Worker %d failed: %s", workerID, err)
				select {
				case workerErrCh <- fmt.Errorf("worker %d failed: %w", workerID, err):
				default:
				}
			}
		}(id)
		id++
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	signal.Notify(signalCh, syscall.SIGTERM)
	signal.Notify(signalCh, syscall.SIGQUIT)

	i := 0

	rateLimiter := rate.NewLimiter(rate.Limit(math.MaxFloat64), math.MaxInt)

	if pr.cfg.MaxSubmissionsPerSecond > 0 {
		rateLimiter = rate.NewLimiter(rate.Limit(pr.cfg.MaxSubmissionsPerSecond), pr.cfg.MaxSubmissionsPerSecond)
	}
	log.Infof("Sending rate: %f per second with %d burst", rateLimiter.Limit(), rateLimiter.Burst())

	// Setup node kill timer if configured
	var nodeKillTimer *time.Timer
	var nodeKillTimerCh <-chan time.Time
	if pr.cfg.NodeKillConfig != nil && pr.cfg.NodeKillConfig.KillInterval > 0 {
		nodeKillTimer = time.NewTimer(pr.cfg.NodeKillConfig.KillInterval)
		nodeKillTimerCh = nodeKillTimer.C
		log.Infof("Node kill timer started, will kill a node after %v", pr.cfg.NodeKillConfig.KillInterval)
		defer func() {
			if nodeKillTimer != nil {
				nodeKillTimer.Stop()
			}
		}()
	}

	var fatalErr error
perfLoop:
	for pr.IsDaemon() || time.Now().Unix() < pr.endTime {
		// If we've been given a maximum number of actions to perform, check if we're done
		if pr.cfg.MaxActions > 0 && int64(getMetricVal(totalActionsCounter)) >= pr.cfg.MaxActions {
			break perfLoop
		}

		select {
		case <-signalCh:
			break perfLoop
		case workerErr := <-workerErrCh:
			fatalErr = workerErr
			log.Errorf("Stopping run due to worker error: %v", workerErr)
			break perfLoop
		case <-nodeKillTimerCh:
			// Clear the timer channel since the timer has fired
			nodeKillTimerCh = nil

			// Select a node to kill
			nodeIndex := rand.Intn(len(pr.nodes))
			log.Infof("Randomly selected node %d (%s) to kill", nodeIndex, pr.cfg.Nodes[nodeIndex].Name)

			// Request pause from all workers
			log.Info("Requesting workers to pause for node kill")
			for i := 0; i < pr.totalWorkers; i++ {
				log.Infof("Sending pause request to worker %d", i)
				pr.pauseRequests[i] <- struct{}{}
				log.Infof("Pause request delivered to worker %d", i)
			}

			// Wait for all workers to acknowledge they've paused
			log.Infof("Waiting for %d workers to acknowledge pause", pr.totalWorkers)
			ackCh := make(chan int, pr.totalWorkers)
			for i := 0; i < pr.totalWorkers; i++ {
				workerID := i
				go func(workerID int) {
					<-pr.pauseAcks[workerID]
					ackCh <- workerID
				}(workerID)
			}
			ackedWorkers := make([]bool, pr.totalWorkers)
			ackedCount := 0
			statusTicker := time.NewTicker(5 * time.Second)
			defer statusTicker.Stop()
			for ackedCount < pr.totalWorkers {
				select {
				case workerID := <-ackCh:
					if !ackedWorkers[workerID] {
						ackedWorkers[workerID] = true
						ackedCount++
						log.Infof("Worker %d acknowledged pause (%d/%d)", workerID, ackedCount, pr.totalWorkers)
					}
				case <-statusTicker.C:
					pendingWorkers := make([]int, 0)
					for i := 0; i < pr.totalWorkers; i++ {
						if !ackedWorkers[i] {
							pendingWorkers = append(pendingWorkers, i)
						}
					}
					log.Warnf("Still waiting for worker pause acknowledgements (%d/%d complete). Pending workers: %v", ackedCount, pr.totalWorkers, pendingWorkers)
				case <-pr.ctx.Done():
					log.Warn("Context cancelled while waiting for workers to acknowledge pause")
					fatalErr = pr.ctx.Err()
					break perfLoop
				}
			}
			log.Info("All workers have paused, proceeding with node kill")

			// Kill the selected node
			if err := pr.nodeManager.KillNode(pr.ctx, nodeIndex); err != nil {
				log.Errorf("Failed to kill node %d: %v", nodeIndex, err)
				// Resume workers even on error
				for i := 0; i < pr.totalWorkers; i++ {
					pr.resumeSignals[i] <- struct{}{}
				}
				break perfLoop
			}

			log.Info("Node killed, waiting for restart")

			// Wait for node restart (blocking in this goroutine)
			restartTimeout := pr.cfg.NodeKillConfig.RestartTimeout
			log.Infof("Waiting for node %d to restart (timeout: %v)", nodeIndex, restartTimeout)
			if err := pr.nodeManager.WaitForNodeRestart(pr.ctx, nodeIndex, restartTimeout); err != nil {
				log.Errorf("Node %d did not restart within timeout: %v", nodeIndex, err)
				// Resume workers even on timeout so test can complete
				for i := 0; i < pr.totalWorkers; i++ {
					pr.resumeSignals[i] <- struct{}{}
				}
				break perfLoop
			}

			log.Infof("Node %d has restarted successfully, resuming workers", nodeIndex)

			// Resume all workers
			for i := 0; i < pr.totalWorkers; i++ {
				pr.resumeSignals[i] <- struct{}{}
			}
			log.Info("All workers have been resumed")

			// Create a new timer after successful restart, so the next kill happens after KillInterval
			if pr.cfg.NodeKillConfig != nil && pr.cfg.NodeKillConfig.KillInterval > 0 {
				if nodeKillTimer != nil {
					nodeKillTimer.Stop()
				}
				nodeKillTimer = time.NewTimer(pr.cfg.NodeKillConfig.KillInterval)
				nodeKillTimerCh = nodeKillTimer.C
				log.Debugf("Node kill timer restarted, next kill will be in %v", pr.cfg.NodeKillConfig.KillInterval)
			}
		case pr.bfr <- i:
			err = rateLimiter.Wait(pr.ctx)
			if err != nil {
				log.Panic(fmt.Errorf("rate limiter failed"))
				break perfLoop
			}
			i++
		case <-pr.ctx.Done():
			pr.cleanup()
			break perfLoop
		}

	}

	pr.stopping = true

	if fatalErr == nil {
		// Wait for all pending transactions to complete
		log.Infof("Waiting up to %v for all pending transactions to complete", pr.cfg.CompletionTimeout)
		deadline := time.Now().Add(pr.cfg.CompletionTimeout)
		checkInterval := 2 * time.Second
		idleStart := time.Now()
		lastEventsCount := int64(getMetricVal(receivedEventsCounter))

		for time.Now().Before(deadline) {
			submissionCount := int64(getMetricVal(totalActionsCounter))
			eventsCount := int64(getMetricVal(receivedEventsCounter))

			if eventsCount >= submissionCount {
				log.Infof("All transactions completed! Submitted: %d, Received: %d", submissionCount, eventsCount)
				break
			}

			// Check if we've been idle (no new events) for too long
			if eventsCount > lastEventsCount {
				// Reset idle start time if there are new events
				idleStart = time.Now()
				lastEventsCount = eventsCount
			} else if time.Since(idleStart) > 30*time.Second {
				// If no new events for 30 seconds, log warning but continue waiting until completion timeout
				log.Warnf("No new events received for 30s. Submitted: %d, Received: %d. Continuing to wait...", submissionCount, eventsCount)
				remainingTransactionIDs := pr.remainingTransactionIDs()
				if len(remainingTransactionIDs) > 0 {
					log.Warnf("Transactions still waiting for events (%d): %v", len(remainingTransactionIDs), remainingTransactionIDs)
				}
				idleStart = time.Now() // Reset to avoid spamming
			}

			log.Infof("Waiting for transactions to complete... Submitted: %d, Received: %d", submissionCount, eventsCount)
			time.Sleep(checkInterval)
		}

		// Final check
		submissionCount := int64(getMetricVal(totalActionsCounter))
		eventsCount := int64(getMetricVal(receivedEventsCounter))
		if eventsCount < submissionCount {
			log.Warnf("Completion timeout reached. Submitted: %d, Received: %d", submissionCount, eventsCount)
		}
	}
	remainingTransactionIDs := pr.remainingTransactionIDs()
	if len(remainingTransactionIDs) > 0 {
		log.Warnf("Transactions still waiting for events (%d): %v", len(remainingTransactionIDs), remainingTransactionIDs)
	}
	log.Info("Running suite post-run verification")
	if postRunErr := pr.currentSuite.PostRun(); postRunErr != nil {
		if fatalErr == nil {
			fatalErr = fmt.Errorf("post-run analysis failed: %w", postRunErr)
		}
		log.Errorf("Post-run analysis failed: %v", postRunErr)
	}

	measuredActions := pr.summary.totalSummary
	measuredTime := time.Since(time.Unix(pr.startTime, 0))

	testName := string(pr.cfg.Test.Name)

	tps := util.GenerateTPS(measuredActions, pr.startTime, pr.endSendTime)
	pr.reportBuilder.AddTestRunMetrics(testName, measuredActions, measuredTime, tps, pr.totalTime)
	err = pr.reportBuilder.GenerateHTML()

	if err != nil {
		log.Errorf("failed to generate performance report: %+v", err)
	}

	// Signal runners to stop, then wait for them with a timeout
	log.Info("Stopping runners (cancelling context and closing WebSockets)")
	pr.shutdown()
	pr.cleanup()

	done := make(chan struct{})
	go func() {
		pr.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Info("All runners stopped")
	case <-time.After(runnerShutdownTimeout):
		log.Warnf("Timeout waiting for runners to stop after %v", runnerShutdownTimeout)
	}

	log.Info("Cleaning up")

	log.Info("Shutdown summary:")
	log.Infof(" - Prometheus metric received_events_total   = %f\n", getMetricVal(receivedEventsCounter))
	log.Infof(" - Prometheus metric incomplete_events_total = %f\n", getMetricVal(incompleteEventsCounter))
	log.Infof(" - Prometheus metric actions_submitted_total = %f\n", getMetricVal(totalActionsCounter))
	log.Infof(" - Test duration: %s", measuredTime)
	log.Infof(" - Measured actions: %d", measuredActions)
	log.Infof(" - Measured send TPS: %2f", tps.SendRate)
	log.Infof(" - Measured throughput: %2f", tps.Throughput)
	log.Infof(" - Measured send duration: %s", pr.sendTime)
	log.Infof(" - Measured event receiving duration: %s", pr.receiveTime)
	log.Infof(" - Measured total duration: %s", pr.totalTime)

	return fatalErr
}

func (pr *perfRunner) cleanup() {
	if pr.currentSuite != nil {
		pr.currentSuite.Unsubscribe()
	}

	// Set flag to indicate websocket is being closed, then close all node WebSockets
	pr.closingWebsocket = true
	for _, node := range pr.nodes {
		if node != nil && node.WSClient != nil {
			node.WSClient.Close()
		}
	}

	if pr.currentSuite != nil {
		pr.currentSuite.Cleanup()
	}
}

func (pr *perfRunner) batchEventLoop(sub rpcclient.Subscription) (err error) {
	defer pr.wg.Done()
	log.Info("Batch Event loop started")
	for {
		log.Trace("blocking until wsconn.Receive or ctx.Done()")
		select {
		// Wait to receive websocket event
		case subNotification, ok := <-sub.Notifications():
			if !ok {
				// Channel closed - check if it's expected (during cleanup) or unexpected
				if pr.closingWebsocket {
					// Expected: cleanup is closing the websocket
					log.Debug("Websocket channel closed during cleanup")
				} else {
					// Unexpected: channel closed but we're not cleaning up
					log.Errorf("Error receiving websocket: channel closed unexpectedly")
				}
				return
			}
			log.Trace("received from websocket")

			// Handle websocket event
			var batch pldapi.TransactionReceiptBatch
			json.Unmarshal(subNotification.GetResult(), &batch)

			if pr.cfg.LogEvents {
				log.Info("Batch: ", string(subNotification.GetResult()))
			}

			g, _ := errgroup.WithContext(pr.ctx)
			g.SetLimit(-1)

			for _, receipt := range batch.Receipts {
				thisReceipt := receipt
				g.Go(func() error {
					transactionID := thisReceipt.ID.String()
					v, ok := pr.workerIDMap.LoadAndDelete(transactionID)
					if !ok {
						// we cant apply reliable filters to ensure we're only getting back receipts
						// for transactions submitted in this test, so just log and skip any we don't
						// recognuse
						log.Warnf("No worker ID map entry for transaction id: %s", transactionID)
						return nil
					}
					workerID := v.(int)

					if pr.cfg.LogEvents {
						eventJSON, _ := json.Marshal(thisReceipt)
						log.Info("Event: ", string(eventJSON))
					}

					receivedEventsCounter.Inc()
					pr.recordCompletedAction()
					// Release worker so it can continue to its next task
					if !pr.stopping {
						if workerID >= 0 {
							prefixedWorkerID := fmt.Sprintf("%s%d", workerPrefix, workerID)
							// No need for locking as channel have built in support
							select {
							case pr.wsReceivers[prefixedWorkerID] <- true:
							default:
							}
						}
					}
					return nil
				})
			}

			// Wait for all go routines to complete
			// The first non-nil go routine will be returned
			// and we will return the error
			log.Debug("Waiting for events from websocket to be handled")
			if err := g.Wait(); err != nil {
				return err
			}
			log.Debug("All events from websocket handled")

			// We have completed all the go routines
			// and can ack the batch
			err := subNotification.Ack(pr.ctx)
			if err != nil {
				log.Errorf("Failed to ack batch receipt: %s", err.Error())
				return err
			}

			pr.summary.mutex.Lock()
			pr.calculateCurrentTps(true)
			pr.summary.mutex.Unlock()
		case <-pr.ctx.Done():
			log.Warnf("Run loop exiting (context cancelled)")
			sub.Unsubscribe(pr.ctx)
			return
		}
	}
}

func (pr *perfRunner) allActionsComplete() bool {
	return pr.cfg.MaxActions > 0 && int64(getMetricVal(totalActionsCounter)) >= pr.cfg.MaxActions
}

func (pr *perfRunner) remainingTransactionIDs() []string {
	transactionIDs := make([]string, 0)
	pr.workerIDMap.Range(func(key, _ any) bool {
		transactionID, ok := key.(string)
		if !ok {
			return true
		}
		transactionIDs = append(transactionIDs, transactionID)
		return true
	})
	sort.Strings(transactionIDs)
	return transactionIDs
}

func (pr *perfRunner) dumpGoroutines(reason string) {
	log.Warnf("Dumping goroutines for diagnostics (%s)", reason)
	if err := pprof.Lookup("goroutine").WriteTo(os.Stderr, 2); err != nil {
		log.Errorf("Failed to dump goroutines (%s): %v", reason, err)
		return
	}
	log.Warnf("Completed goroutine dump (%s)", reason)
}

func (pr *perfRunner) runLoop(tc testsuite.TestCase, workerID int, actionsPerLoop int) error {
	defer pr.wg.Done()
	testName := tc.Name()
	preFixedWorkerID := fmt.Sprintf("%s%d", workerPrefix, workerID)

	loop := 0

	for {
		select {
		case <-pr.pauseRequests[workerID]:
			// Worker received pause request - send acknowledgment
			log.Infof("Worker %d received pause request", workerID)
			pr.pauseAcks[workerID] <- struct{}{}
			log.Infof("Worker %d acknowledged pause, waiting for resume signal", workerID)

			// Wait for resume signal
			<-pr.resumeSignals[workerID]
			log.Infof("Worker %d received resume signal", workerID)

			// Continue to next iteration of loop
			continue
		case <-pr.bfr:
			var actionsCompleted int

			// Worker sends its task
			hist, histErr := perfTestDurationHistogram.GetMetricWith(prometheus.Labels{
				"test": string(testName),
			})

			if histErr != nil {
				log.Errorf("Error retrieving histogram: %s", histErr)
			}

			startTime := time.Now()

			type ActionResponse struct {
				transactionID string
				err           error
			}

			actionResponses := make(chan *ActionResponse, actionsPerLoop)

			var sentTime time.Time
			var submissionSecondsPerLoop float64
			var eventReceivingSecondsPerLoop float64
			transactionIDs := make([]string, 0)

			pendingActions := 0
			for actionsCompleted = 0; actionsCompleted < actionsPerLoop; actionsCompleted++ {

				if pr.allActionsComplete() {
					break
				}
				actionCount := actionsCompleted
				pendingActions++
				go func() {
					transactionID, err := tc.RunOnce(actionCount)
					log.Debugf("%d --> %s action %d sent after %f seconds", workerID, testName, actionCount, time.Since(startTime).Seconds())
					actionResponses <- &ActionResponse{
						transactionID: transactionID,
						err:           err,
					}
				}()
			}
			resultCount := 0
			pendingActionsTicker := time.NewTicker(10 * time.Second)
			waitForActionsStart := time.Now()
			goroutineDumpedForPendingActions := false
			for pendingActions > 0 {
				select {
				case <-pr.ctx.Done():
					pendingActionsTicker.Stop()
					return nil
				case <-pendingActionsTicker.C:
					log.Infof(
						"%d --> %s Still waiting for pending action responses (pending=%d received=%d elapsed=%s)",
						workerID,
						testName,
						pendingActions,
						resultCount,
						time.Since(waitForActionsStart).Round(time.Second),
					)
					if !goroutineDumpedForPendingActions {
						pr.dumpGoroutines(fmt.Sprintf("worker %d pending action responses", workerID))
						goroutineDumpedForPendingActions = true
					}
				case aResponse := <-actionResponses:
					pendingActions--
					resultCount++
					if aResponse.err != nil {
						pendingActionsTicker.Stop()
						return aResponse.err
					}
					transactionIDs = append(transactionIDs, aResponse.transactionID)
					pr.workerIDMap.Store(aResponse.transactionID, workerID)
					log.Debugf("%d --> %s Sent transaction ID: %s", workerID, testName, aResponse.transactionID)
					totalActionsCounter.Inc()
				}
			}
			pendingActionsTicker.Stop()
			// if we've reached the expected amount of metadata calls then stop
			if resultCount == actionsPerLoop {
				submissionDurationPerLoop := time.Since(startTime)
				pr.sendTime.Record(submissionDurationPerLoop)
				submissionSecondsPerLoop = submissionDurationPerLoop.Seconds()
				sentTime = time.Now()
				log.Debugf("%d --> %s All actions sent %d after %f seconds", workerID, testName, resultCount, submissionSecondsPerLoop)

				pr.endSendTime = time.Now().Unix()
			}
			// Wait for worker to confirm each message before proceeding to next task
			if !pr.cfg.NoWaitSubmission {
				for j := 0; j < actionsCompleted; j++ {
					select {
					case <-pr.ctx.Done():
						return nil
					case <-pr.wsReceivers[preFixedWorkerID]:
						if len(transactionIDs) > 0 {
							transactionIDs = transactionIDs[1:]
						}
					}
				}
			}
			totalDurationPerLoop := time.Since(startTime)
			pr.totalTime.Record(totalDurationPerLoop)
			secondsPerLoop := totalDurationPerLoop.Seconds()

			eventReceivingDurationPerLoop := time.Since(sentTime)
			eventReceivingSecondsPerLoop = eventReceivingDurationPerLoop.Seconds()
			pr.receiveTime.Record(totalDurationPerLoop)

			total := submissionSecondsPerLoop + eventReceivingSecondsPerLoop
			subPortion := int((submissionSecondsPerLoop / total) * 100)
			envPortion := int((eventReceivingSecondsPerLoop / total) * 100)
			log.Infof("%d <-- %s Finished (loop=%d), submission time: %f s, event receive time: %f s. Ratio (%d/%d) after %f seconds", workerID, testName, loop, submissionSecondsPerLoop, eventReceivingSecondsPerLoop, subPortion, envPortion, secondsPerLoop)

			if histErr == nil {
				log.Debugf("%d <-- %s Emmiting (loop=%d) after %f seconds", workerID, testName, loop, secondsPerLoop)

				hist.Observe(secondsPerLoop)
			}
			loop++
		case <-pr.ctx.Done():
			return nil
		}
	}
}

func (pr *perfRunner) recordCompletedAction() {
	if pr.ramping() {
		_ = atomic.AddInt64(&pr.summary.rampSummary, 1)
	} else {
		_ = atomic.AddInt64(&pr.summary.totalSummary, 1)
	}
}

func (pr *perfRunner) IsDaemon() bool {
	return pr.cfg.Daemon
}

func (pr *perfRunner) calculateCurrentTps(logValue bool) float64 {
	// If we're still ramping, give the current rate during the ramp
	// If we're done ramping, calculate TPS from the end of the ramp onward
	var startTime int64
	var measuredActions int64
	if pr.ramping() {
		measuredActions = pr.summary.rampSummary
		startTime = pr.startRampTime
	} else {
		measuredActions = pr.summary.totalSummary
		startTime = pr.startTime
	}
	duration := time.Since(time.Unix(startTime, 0)).Seconds()
	currentTps := float64(measuredActions) / duration
	if logValue {
		log.Infof("Current TPS: %v Measured Actions: %v Duration: %v", currentTps, measuredActions, duration)
	}
	return currentTps
}

func (pr *perfRunner) ramping() bool {
	if time.Now().Before(time.Unix(pr.endRampTime, 0)) {
		return true
	}
	return false
}
