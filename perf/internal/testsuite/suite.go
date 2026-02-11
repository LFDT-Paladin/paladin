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

package testsuite

import (
	"context"

	"github.com/LFDT-Paladin/paladin/perf/internal/conf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldclient"
)

// GetTestSuite returns a new TestSuite for the given test name with the given context and clients, or nil if unknown.
func GetTestSuite(name conf.TestName, ctx context.Context, httpClients []pldclient.PaladinClient, wsClient pldclient.PaladinWSClient, nodes []conf.NodeConfig) TestSuite {
	switch name {
	case conf.PerfTestPublicContract:
		return NewPublicContractSuite(ctx, httpClients, wsClient, nodes)
	case conf.PerfTestPrivateTransactionNodeRestart:
		return NewPrivateTransactionNodeRestartSuite(ctx, httpClients, wsClient, nodes)
	default:
		return nil
	}
}
