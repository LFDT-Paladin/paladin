// Copyright © 2026 Kaleido, Inc.
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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	log "github.com/sirupsen/logrus"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/test/internal/testsuite"
)

// sequencerStageSnapshot holds cumulative histogram data for one sequencer stage label.
// SumMs is the cumulative sum of time spent in this stage across all observed transactions
// (in milliseconds, matching the histogram bucket unit). Diff consecutive snapshots to get
// interval rates.
type sequencerStageSnapshot struct {
	Stage       string  `json:"stage"`
	SampleCount uint64  `json:"sampleCount"`
	SumMs       float64 `json:"sumMs"`
}

// nodeDiagnosticSnapshot holds all collected metrics for one node at one point in time.
type nodeDiagnosticSnapshot struct {
	NodeName string `json:"nodeName"`

	// Go runtime metrics from Prometheus /metrics (zero if metricsEndpoint not configured)
	GoGoroutines   float64 `json:"goGoroutines"`
	HeapAllocBytes float64 `json:"heapAllocBytes"`
	HeapInuseBytes float64 `json:"heapInuseBytes"`
	ProcessOpenFDs float64 `json:"processOpenFds"`

	// Sequencer metrics from Prometheus /metrics
	ActiveSequencers         float64                  `json:"activeSequencers"`
	CoordinatingTransactions float64                  `json:"coordinatingTransactions"`
	SequencerStages          []sequencerStageSnapshot `json:"sequencerStages,omitempty"`

	// Highest sequence number seen in the reliable message log for this node.
	// Diff consecutive snapshots to measure transport message creation rate and
	// detect DB table growth that may slow per-transaction processing.
	MaxReliableMessageSeq uint64 `json:"maxReliableMessageSeq"`

	// Non-empty if the Prometheus scrape failed.
	MetricsError string `json:"metricsError,omitempty"`
	// Non-empty if the transport RPC query failed.
	TransportError string `json:"transportError,omitempty"`
}

// diagnosticSnapshot is one complete sample written as metrics.json in a per-tick directory.
type diagnosticSnapshot struct {
	Timestamp      time.Time                 `json:"timestamp"`
	ElapsedSeconds float64                   `json:"elapsedSeconds"`
	Nodes          []*nodeDiagnosticSnapshot `json:"nodes"`
}

// createDiagnosticsDir creates diagnostics/<testname>-<timestamp>/ and returns the path.
// The parent diagnostics/ directory is created if it does not already exist.
func (pr *perfRunner) createDiagnosticsDir() (string, error) {
	testName := string(pr.cfg.Test.Name)
	timestamp := time.Now().Format("20060102-150405")
	runDir := filepath.Join("diagnostics", fmt.Sprintf("%s-%s", testName, timestamp))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create diagnostics directory %s: %w", runDir, err)
	}
	log.Infof("Node diagnostics will be written to: %s (interval: %s)", runDir, pr.cfg.Diagnostics.Interval)
	return runDir, nil
}

// runDiagnosticsTicker fires at the configured interval. On each tick it creates a
// tick-<elapsed>/ subdirectory containing metrics.json and per-node goroutine dump files.
// It exits when stopRunners is closed.
func (pr *perfRunner) runDiagnosticsTicker(dir string) {
	defer pr.wg.Done()

	ticker := time.NewTicker(pr.cfg.Diagnostics.Interval)
	defer ticker.Stop()

	testStart := time.Unix(pr.startTime, 0)

	for {
		select {
		case <-pr.stopRunners:
			return
		case <-ticker.C:
			tickDir := filepath.Join(dir, time.Now().Format("20060102-150405"))
			if err := os.MkdirAll(tickDir, 0o755); err != nil {
				log.Warnf("Failed to create tick directory %s: %v", tickDir, err)
				continue
			}
			pr.writeMetrics(tickDir, testStart)
			pr.collectDebugProfiles(tickDir)
		}
	}
}

func (pr *perfRunner) writeMetrics(tickDir string, testStart time.Time) {
	now := time.Now()
	snap := &diagnosticSnapshot{
		Timestamp:      now,
		ElapsedSeconds: now.Sub(testStart).Seconds(),
		Nodes:          make([]*nodeDiagnosticSnapshot, 0, len(pr.nodes)),
	}
	for _, node := range pr.nodes {
		snap.Nodes = append(snap.Nodes, collectNodeMetrics(pr.ctx, node))
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		log.Warnf("Failed to marshal metrics snapshot: %v", err)
		return
	}
	metricsPath := filepath.Join(tickDir, "metrics.json")
	if err := os.WriteFile(metricsPath, data, 0o644); err != nil {
		log.Warnf("Failed to write %s: %v", metricsPath, err)
	} else {
		log.Infof("Wrote diagnostics metrics: %s", metricsPath)
	}
}

// collectNodeMetrics scrapes Prometheus and queries the transport layer for one node.
// Each collection is bounded by a 30-second timeout so a slow or unreachable node does
// not stall the ticker goroutine.
func collectNodeMetrics(ctx context.Context, node *testsuite.Node) *nodeDiagnosticSnapshot {
	snap := &nodeDiagnosticSnapshot{NodeName: node.Config.Name}

	collectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if node.Config.MetricsEndpoint != "" {
		mf, err := scrapePrometheusMetrics(collectCtx, node.Config.MetricsEndpoint)
		if err != nil {
			snap.MetricsError = err.Error()
		} else {
			snap.GoGoroutines = gaugeValue(mf, "go_goroutines")
			snap.HeapAllocBytes = gaugeValue(mf, "go_memstats_heap_alloc_bytes")
			snap.HeapInuseBytes = gaugeValue(mf, "go_memstats_heap_inuse_bytes")
			snap.ProcessOpenFDs = gaugeValue(mf, "process_open_fds")
			snap.ActiveSequencers = gaugeValue(mf, "distributed_sequencer_active_sequencers")
			snap.CoordinatingTransactions = gaugeValue(mf, "distributed_sequencer_coordinating_txns")
			snap.SequencerStages = histogramByLabel(mf, "distributed_sequencer_sequencer_stage", "stage")
		}
	}

	msgs, err := node.HTTPClient.Transport().QueryReliableMessages(
		collectCtx,
		query.NewQueryBuilder().Sort("-sequence").Limit(1).Query(),
	)
	if err != nil {
		snap.TransportError = err.Error()
	} else if len(msgs) > 0 {
		snap.MaxReliableMessageSeq = msgs[0].Sequence
	}

	return snap
}

// collectDebugProfiles starts kubectl port-forwards for the duration of the collection,
// fetches goroutine dumps and heap profiles from each node, then stops the port-forwards.
func (pr *perfRunner) collectDebugProfiles(tickDir string) {
	cmds, err := startDebugPortForwards(pr.ctx, pr.nodes)
	if err != nil {
		log.Warnf("Debug port-forward setup failed, skipping pprof collection: %v", err)
		stopDebugPortForwards(cmds)
		return
	}
	defer stopDebugPortForwards(cmds)

	pr.writeGoroutineDumps(tickDir)
	pr.writeHeapProfiles(tickDir)
}

// writeGoroutineDumps fetches /debug/pprof/goroutine?debug=2 from each node that has a
// DebugPortForward configured and writes the output to goroutines-<nodename>.txt inside tickDir.
// All nodes are fetched concurrently.
func (pr *perfRunner) writeGoroutineDumps(tickDir string) {
	var wg sync.WaitGroup
	for _, node := range pr.nodes {
		debugURL := nodeDebugURL(node)
		if debugURL == "" {
			continue
		}
		node, debugURL := node, debugURL
		wg.Add(1)
		go func() {
			defer wg.Done()
			filename := filepath.Join(tickDir, fmt.Sprintf("goroutines-%s.txt", node.Config.Name))
			if err := fetchGoroutineDump(pr.ctx, debugURL, filename); err != nil {
				log.Warnf("Goroutine dump failed for %s: %v", node.Config.Name, err)
			} else {
				log.Infof("Wrote goroutine dump: %s", filename)
			}
		}()
	}
	wg.Wait()
}

// writeHeapProfiles fetches the pprof heap profile from each node that has a
// DebugPortForward configured and writes the binary profile to heap-<nodename>.prof inside tickDir.
// All nodes are fetched concurrently.
// Profiles can be analysed with: go tool pprof heap-<node>.prof
// Ticks can be diffed with:      go tool pprof -base tick-A/heap-<node>.prof tick-B/heap-<node>.prof
func (pr *perfRunner) writeHeapProfiles(tickDir string) {
	var wg sync.WaitGroup
	for _, node := range pr.nodes {
		debugURL := nodeDebugURL(node)
		if debugURL == "" {
			continue
		}
		node, debugURL := node, debugURL
		wg.Add(1)
		go func() {
			defer wg.Done()
			filename := filepath.Join(tickDir, fmt.Sprintf("heap-%s.prof", node.Config.Name))
			if err := fetchPprofProfile(pr.ctx, debugURL, "/debug/pprof/heap", filename); err != nil {
				log.Warnf("Heap profile failed for %s: %v", node.Config.Name, err)
			} else {
				log.Infof("Wrote heap profile: %s", filename)
			}
		}()
	}
	wg.Wait()
}

// fetchGoroutineDump fetches the pprof goroutine text dump from debugEndpoint and writes
// it to filename.
func fetchGoroutineDump(ctx context.Context, debugEndpoint, filename string) error {
	return fetchPprofProfile(ctx, debugEndpoint, "/debug/pprof/goroutine?debug=2", filename)
}

// fetchPprofProfile performs a GET against debugEndpoint+path and streams the response
// body to filename. Used for both text (goroutine) and binary (heap) pprof endpoints.
func fetchPprofProfile(ctx context.Context, debugEndpoint, path, filename string) error {
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	url := strings.TrimRight(debugEndpoint, "/") + path
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create %s: %w", filename, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write to %s: %w", filename, err)
	}
	return nil
}

// scrapePrometheusMetrics fetches and parses the Prometheus text-format /metrics page
// from the given base URL (e.g. "http://node1:6100").
func scrapePrometheusMetrics(ctx context.Context, endpoint string) (map[string]*dto.MetricFamily, error) {
	url := strings.TrimRight(endpoint, "/") + "/metrics"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("Accept", "text/plain; version=0.0.4")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scrape %s: %w", url, err)
	}
	defer resp.Body.Close()

	var parser expfmt.TextParser
	mf, parseErr := parser.TextToMetricFamilies(resp.Body)
	// TextToMetricFamilies can return partial results alongside an error
	// (e.g. for duplicate metric names). Use whatever was parsed.
	if parseErr != nil && len(mf) == 0 {
		return nil, fmt.Errorf("parse metrics from %s: %w", url, parseErr)
	}
	return mf, nil
}

// gaugeValue extracts the current value of a simple gauge metric family.
func gaugeValue(mf map[string]*dto.MetricFamily, name string) float64 {
	family, ok := mf[name]
	if !ok || len(family.GetMetric()) == 0 {
		return 0
	}
	g := family.GetMetric()[0].GetGauge()
	if g == nil {
		return 0
	}
	return g.GetValue()
}

// histogramByLabel extracts cumulative histogram data from a metric family that uses
// a single label to distinguish series (e.g. stage="Coord_Assembling").
func histogramByLabel(mf map[string]*dto.MetricFamily, name, labelName string) []sequencerStageSnapshot {
	family, ok := mf[name]
	if !ok {
		return nil
	}
	stages := make([]sequencerStageSnapshot, 0, len(family.GetMetric()))
	for _, m := range family.GetMetric() {
		h := m.GetHistogram()
		if h == nil {
			continue
		}
		labelVal := ""
		for _, lp := range m.GetLabel() {
			if lp.GetName() == labelName {
				labelVal = lp.GetValue()
				break
			}
		}
		stages = append(stages, sequencerStageSnapshot{
			Stage:       labelVal,
			SampleCount: h.GetSampleCount(),
			SumMs:       h.GetSampleSum(),
		})
	}
	return stages
}
