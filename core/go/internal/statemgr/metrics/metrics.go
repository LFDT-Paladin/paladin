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
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var METRICS_SUBSYSTEM = "state_manager"

// operationBuckets are hand-picked rather than exponential, weighting resolution toward the
// sub-millisecond range where these fast queries actually vary. They match the distributed_sequencer
// operation buckets, so a query timing compares bucket for bucket against the domain call around it.
var operationBuckets = []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type StateManagerMetrics interface {
	// ObserveFindAvailableStates records the wall time of one FindAvailableStates call as the engine
	// serves it: the DB read plus the in-memory merge of remote-view candidates and spent-state
	// exclusion. It brackets that one entrypoint, so find_available_states_duration_seconds_count is a
	// count of available-state queries and can be read against
	// domain_call_duration_seconds_count{method=assemble} as queries per assemble.
	ObserveFindAvailableStates(domain string, d time.Duration)
	// ObserveStateQueryDB records the wall time of the DB round-trip alone (the pgx/Postgres query that
	// Go profiles cannot see past), for every state read the store makes rather than only the available
	// ones. An available-state query's round-trip falls inside its ObserveFindAvailableStates bracket.
	ObserveStateQueryDB(domain string, d time.Duration)
}

type stateManagerMetrics struct {
	findAvailableStates *prometheus.HistogramVec
	stateQueryDB        *prometheus.HistogramVec
}

func InitMetrics(ctx context.Context, registry *prometheus.Registry) *stateManagerMetrics {
	m := &stateManagerMetrics{
		findAvailableStates: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "find_available_states_duration_seconds",
			Help:      "Wall time of one FindAvailableStates call (DB read + remote-view merge + spent-state exclusion)",
			Subsystem: METRICS_SUBSYSTEM, Buckets: operationBuckets}, []string{"domain"}),
		stateQueryDB: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "state_query_db_duration_seconds",
			Help:      "Wall time of the DB round-trip inside any state read (the pgx/Postgres layer profiles cannot see)",
			Subsystem: METRICS_SUBSYSTEM, Buckets: operationBuckets}, []string{"domain"}),
	}
	registry.MustRegister(m.findAvailableStates)
	registry.MustRegister(m.stateQueryDB)
	return m
}

// Seconds carries the full float64 resolution of the duration, so sub-millisecond round-trips land in
// the lower buckets instead of truncating away.
func (m *stateManagerMetrics) ObserveFindAvailableStates(domain string, d time.Duration) {
	m.findAvailableStates.WithLabelValues(domain).Observe(d.Seconds())
}

func (m *stateManagerMetrics) ObserveStateQueryDB(domain string, d time.Duration) {
	m.stateQueryDB.WithLabelValues(domain).Observe(d.Seconds())
}
