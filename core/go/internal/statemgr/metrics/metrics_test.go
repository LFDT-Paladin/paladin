// Copyright contributors to Paladin, an LFDT project
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

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitMetricsRegistersAndObserves(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := InitMetrics(context.Background(), registry)

	m.ObserveFindAvailableStates("domain1", 3*time.Millisecond)
	m.ObserveStateQueryDB("domain1", 2*time.Millisecond)

	for name, expected := range map[string]int{
		"state_manager_find_available_states_duration_seconds": 1,
		"state_manager_state_query_db_duration_seconds":        1,
	} {
		assert.Equal(t, expected, testutil.CollectAndCount(registry, name), name)
	}
}

func TestInitMetricsRegistersOnlyOnce(t *testing.T) {
	registry := prometheus.NewRegistry()
	require.NotNil(t, InitMetrics(context.Background(), registry))
	assert.Panics(t, func() { InitMetrics(context.Background(), registry) })
}
