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
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/LFDT-Paladin/paladin/test/internal/testsuite"
)

// nodeDebugURL returns the base URL of the debug/pprof server for a node, or empty
// string if no DebugPortForward is configured.
func nodeDebugURL(node *testsuite.Node) string {
	pf := node.Config.DebugPortForward
	if pf == nil || pf.LocalPort == 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", pf.LocalPort)
}

// startDebugPortForwards starts a kubectl port-forward for every node that has a
// DebugPortForward config. All processes are launched concurrently and the function
// blocks until every forward is ready or the context is cancelled.
// The returned slice must be passed to stopDebugPortForwards during cleanup.
func startDebugPortForwards(ctx context.Context, nodes []*testsuite.Node) ([]*exec.Cmd, error) {
	type result struct {
		cmd *exec.Cmd
		err error
	}

	var chs []chan result

	for _, node := range nodes {
		pf := node.Config.DebugPortForward
		if pf == nil || pf.LocalPort == 0 || pf.RemotePort == 0 || pf.Service == "" {
			continue
		}

		ch := make(chan result, 1)
		chs = append(chs, ch)

		node, pf := node, pf // capture loop variables
		go func() {
			namespace := pf.Namespace
			if namespace == "" {
				namespace = "default"
			}
			portMapping := fmt.Sprintf("%d:%d", pf.LocalPort, pf.RemotePort)
			target := fmt.Sprintf("svc/%s", pf.Service)

			log.Infof("Starting kubectl port-forward for %s: %s %s", node.Config.Name, target, portMapping)
			cmd := exec.CommandContext(ctx, "kubectl", "port-forward", "-n", namespace, target, portMapping) //nolint:gosec
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Start(); err != nil {
				ch <- result{nil, fmt.Errorf("node %s: failed to start kubectl port-forward: %w", node.Config.Name, err)}
				return
			}

			debugURL := nodeDebugURL(node)
			if err := waitForDebugEndpoint(ctx, debugURL, node.Config.Name); err != nil {
				ch <- result{cmd, err}
				return
			}
			log.Infof("Debug port-forward ready for %s at %s", node.Config.Name, debugURL)
			ch <- result{cmd, nil}
		}()
	}

	var cmds []*exec.Cmd
	var firstErr error
	for _, ch := range chs {
		r := <-ch
		if r.cmd != nil {
			cmds = append(cmds, r.cmd)
		}
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
	}
	return cmds, firstErr
}

// stopDebugPortForwards kills all port-forward processes concurrently.
func stopDebugPortForwards(cmds []*exec.Cmd) {
	var wg sync.WaitGroup
	for _, cmd := range cmds {
		if cmd.Process == nil {
			continue
		}
		cmd := cmd
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := cmd.Process.Kill(); err != nil {
				log.Debugf("Failed to kill port-forward process (pid %d): %v", cmd.Process.Pid, err)
			}
			_ = cmd.Wait()
		}()
	}
	wg.Wait()
}

// waitForDebugEndpoint polls /debug/pprof/ on the given base URL until it returns
// HTTP 200 or the context is cancelled. Gives up after 15 seconds.
func waitForDebugEndpoint(ctx context.Context, baseURL, nodeName string) error {
	probeURL := baseURL + "/debug/pprof/"
	deadline := time.Now().Add(15 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := client.Get(probeURL) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("node %s: debug endpoint %s did not become ready within 15s", nodeName, baseURL)
}
