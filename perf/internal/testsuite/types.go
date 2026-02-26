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

package testsuite

import (
	"context"

	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
)

// Node holds the HTTP client, WebSocket client, and config for one configured node.
type Node struct {
	HTTPClient pldclient.PaladinClient
	WSClient   pldclient.PaladinWSClient
	Config     conf.NodeConfig
}

// Runner is implemented by the perf runner; suites use it to get configured nodes and clients.
type Runner interface {
	GetNodes() []*Node
}

// TestCase is the per-worker object that runs one loop of the test.
type TestCase interface {
	Name() conf.TestName
	RunOnce(iterationCount int) (trackingID string, err error)
}

// TestSuite is the one-per-test-type object: it runs setup once and creates workers per loop.
type TestSuite interface {
	Setup() error
	Subscribe() (rpcclient.Subscription, error)
	NewWorker(startTime int64, workerID int) TestCase
	PostRun() error
	Unsubscribe()
	Cleanup()
}

type testBase struct {
	ctx       context.Context
	startTime int64
	workerID  int
}
