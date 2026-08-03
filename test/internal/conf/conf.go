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

package conf

import (
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
)

type RunnerConfig struct {
	LogLevel                string
	Test                    TestCaseConfig
	Length                  time.Duration
	WSConfig                pldconf.WSClientConfig
	HTTPConfig              pldconf.HTTPClientConfig
	Daemon                  bool
	LogEvents               bool
	MaxActions              int64
	RampLength              time.Duration
	MaxSubmissionsPerSecond int
	CompletionTimeout       time.Duration
	NoWaitSubmission        bool
	RollingCheckBatchSize   int
	NodeKillConfig          *NodeKillConfig
	Diagnostics             *DiagnosticsConfig
	Nodes                   []NodeConfig
}

type PerformanceTestConfig struct {
	LogLevel   string                   `json:"logLevel" yaml:"logLevel"`
	Instances  []InstanceConfig         `json:"instances" yaml:"instances"`
	WSConfig   pldconf.WSClientConfig   `json:"wsConfig,omitempty" yaml:"wsConfig,omitempty"`
	HTTPConfig pldconf.HTTPClientConfig `json:"httpConfig,omitempty" yaml:"httpConfig,omitempty"`
	Daemon     bool                     `json:"daemon,omitempty" yaml:"daemon,omitempty"`
	Nodes      []NodeConfig             `json:"nodes" yaml:"nodes"`
	LogEvents  bool                     `json:"logEvents,omitempty" yaml:"logEvents,omitempty"`
}

type InstanceConfig struct {
	Name                    string             `json:"name" yaml:"name"`
	Test                    TestCaseConfig     `json:"test" yaml:"test"`
	Length                  time.Duration      `json:"length" yaml:"length"`
	MaxActions              int64              `json:"maxActions,omitempty" yaml:"maxActions,omitempty"`
	RampLength              time.Duration      `json:"rampLength,omitempty" yaml:"rampLength,omitempty"`
	MaxSubmissionsPerSecond int                `json:"maxSubmissionsPerSecond" yaml:"maxSubmissionsPerSecond"`
	CompletionTimeout       time.Duration      `json:"completionTimeout,omitempty" yaml:"completionTimeout,omitempty"`
	NoWaitSubmission        bool               `json:"noWaitSubmission,omitempty" yaml:"noWaitSubmission,omitempty"`
	RollingCheckBatchSize   int                `json:"rollingCheckBatchSize,omitempty" yaml:"rollingCheckBatchSize,omitempty"`
	NodeKillConfig          *NodeKillConfig    `json:"nodeKillConfig,omitempty" yaml:"nodeKillConfig,omitempty"`
	Diagnostics             *DiagnosticsConfig `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
}

type TestCaseConfig struct {
	Name           TestName       `json:"name" yaml:"name"`
	Workers        int            `json:"workers" yaml:"workers"`
	ActionsPerLoop int            `json:"actionsPerLoop" yaml:"actionsPerLoop"`
	Options        map[string]any `json:"options,omitempty" yaml:"options,omitempty"`
}

// DebugPortForwardConfig instructs the test runner to set up a kubectl port-forward
// directly to the named pod's debug/pprof server before collecting each diagnostics
// snapshot. Port-forwarding to the pod (rather than the service) is required because
// the debug port is not declared in the service spec.
type DebugPortForwardConfig struct {
	Namespace  string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Pod        string `json:"pod,omitempty" yaml:"pod,omitempty"`
	RemotePort int    `json:"remotePort,omitempty" yaml:"remotePort,omitempty"`
	LocalPort  int    `json:"localPort,omitempty" yaml:"localPort,omitempty"`
}

type NodeConfig struct {
	Name             string                  `json:"name" yaml:"name"`
	HTTPEndpoint     string                  `json:"httpEndpoint" yaml:"httpEndpoint"`
	WSEndpoint       string                  `json:"wsEndpoint" yaml:"wsEndpoint"`
	MetricsEndpoint  string                  `json:"metricsEndpoint,omitempty" yaml:"metricsEndpoint,omitempty"`
	DebugPortForward *DebugPortForwardConfig `json:"debugPortForward,omitempty" yaml:"debugPortForward,omitempty"`
}

// DiagnosticsConfig enables periodic collection of Paladin node metrics.
// Set Interval to a non-zero duration to activate. On each tick a timestamped
// subdirectory is written under diagnostics/<testname>-<timestamp>/ containing
// metrics.json and per-node goroutine dump and heap profile files.
// Nodes with MetricsEndpoint set are Prometheus-scraped; nodes with DebugPortForward
// set have goroutine stacks and heap profiles fetched via /debug/pprof.
type DiagnosticsConfig struct {
	Interval time.Duration `json:"interval,omitempty" yaml:"interval,omitempty"`
}

type NodeKillConfig struct {
	KillCommandTemplate string        `json:"killCommandTemplate,omitempty" yaml:"killCommandTemplate,omitempty"`
	HealthCheckCommand  string        `json:"healthCheckCommand,omitempty" yaml:"healthCheckCommand,omitempty"`
	HealthCheckTemplate string        `json:"healthCheckTemplate,omitempty" yaml:"healthCheckTemplate,omitempty"`
	RestartTimeout      time.Duration `json:"restartTimeout,omitempty" yaml:"restartTimeout,omitempty"`
	KillInterval        time.Duration `json:"killInterval,omitempty" yaml:"killInterval,omitempty"`
}

type TestName string

const (
	// PerfTestPublicContract invokes a public smart contract and checks for transaction receipts
	PerfTestPublicContract TestName = "public_contract"
	// PerfTestPrivacyGroupContractDeploy creates a privacy group and deploys a contract per run
	PerfTestPrivacyGroupContractDeploy TestName = "privacy_group_contract_deploy"
	// PerfTestPrivateTransactionNodeRestart drives pente transactions across nodes, kills a node, and verifies recovery
	PerfTestPrivateTransactionNodeRestart TestName = "private_transaction_node_restart"
	// PerfTestNotoRevertableHooks deploys Noto with Pente hooks that include a revertable external call, verifying receipts contain revert data
	PerfTestNotoRevertableHooks TestName = "noto_revertable_hooks"
	// PerfTestNotoTransfer deploys Noto in basic notary mode and drives token transfers, rotating recipients across all configured nodes
	PerfTestNotoTransfer TestName = "noto_transfer"
	// PerfTestNotoPenteTracker deploys Noto with a Pente privacy group using NotoTrackerERC20 as the hook, then drives token transfers across all nodes
	PerfTestNotoPenteTracker TestName = "noto_pente_tracker"
)
