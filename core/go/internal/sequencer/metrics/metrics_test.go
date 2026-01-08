/*
 * Copyright © 2025 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestIncAcceptedTransactions(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.IncAcceptedTransactions()
	metrics.IncAcceptedTransactions()
	metrics.IncAcceptedTransactions()

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the accepted transactions metric
	var acceptedMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_accepted_txns_total" {
			acceptedMetric = mf
			break
		}
	}

	assert.NotNil(t, acceptedMetric, "accepted_txns_total metric should exist")
	assert.Equal(t, acceptedMetric.GetMetric()[0].GetCounter().GetValue(), float64(3))
}

func TestIncAssembledTransactions(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.IncAssembledTransactions()
	metrics.IncAssembledTransactions()

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the assembled transactions metric
	var assembledMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_assembled_txns_total" {
			assembledMetric = mf
			break
		}
	}

	assert.NotNil(t, assembledMetric, "assembled_txns_total metric should exist")
	assert.Equal(t, assembledMetric.GetMetric()[0].GetCounter().GetValue(), float64(2))
}

func TestIncEndorsedTransactions(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.IncEndorsedTransactions()
	metrics.IncEndorsedTransactions()
	metrics.IncEndorsedTransactions()
	metrics.IncEndorsedTransactions()

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the endorsed transactions metric
	var endorsedMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_endorsed_txns_total" {
			endorsedMetric = mf
			break
		}
	}

	assert.NotNil(t, endorsedMetric, "endorsed_txns_total metric should exist")
	assert.Equal(t, endorsedMetric.GetMetric()[0].GetCounter().GetValue(), float64(4))
}

func TestIncDispatchedTransactions(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.IncDispatchedTransactions()
	metrics.IncDispatchedTransactions()
	metrics.IncDispatchedTransactions()
	metrics.IncDispatchedTransactions()
	metrics.IncDispatchedTransactions()

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the dispatched transactions metric
	var dispatchedMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_dispatched_txns_total" {
			dispatchedMetric = mf
			break
		}
	}

	assert.NotNil(t, dispatchedMetric, "dispatched_txns_total metric should exist")
	assert.Equal(t, dispatchedMetric.GetMetric()[0].GetCounter().GetValue(), float64(5))
}

func TestIncConfirmedTransactions(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.IncConfirmedTransactions()
	metrics.IncConfirmedTransactions()

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the confirmed transactions metric
	var confirmedMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_confirmed_txns_total" {
			confirmedMetric = mf
			break
		}
	}

	assert.NotNil(t, confirmedMetric, "confirmed_txns_total metric should exist")
	assert.Equal(t, confirmedMetric.GetMetric()[0].GetCounter().GetValue(), float64(2))
}

func TestIncRevertedTransactions(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.IncRevertedTransactions()
	metrics.IncRevertedTransactions()
	metrics.IncRevertedTransactions()

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the reverted transactions metric
	var revertedMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_reverted_txns_total" {
			revertedMetric = mf
			break
		}
	}

	assert.NotNil(t, revertedMetric, "reverted_txns_total metric should exist")
	assert.Equal(t, revertedMetric.GetMetric()[0].GetCounter().GetValue(), float64(3))
}

func TestSetActiveCoordinators(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.SetActiveCoordinators(5)
	metrics.SetActiveCoordinators(10)
	metrics.SetActiveCoordinators(3)

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the active coordinators metric
	var coordinatorsMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_active_coordinators" {
			coordinatorsMetric = mf
			break
		}
	}

	assert.NotNil(t, coordinatorsMetric, "active_coordinators metric should exist")
	assert.Equal(t, coordinatorsMetric.GetMetric()[0].GetGauge().GetValue(), float64(3))
}

func TestDecCoordinatingTransactions(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	// First increment to set a baseline
	metrics.IncCoordinatingTransactions()
	metrics.IncCoordinatingTransactions()
	metrics.IncCoordinatingTransactions()

	// Then decrement
	metrics.DecCoordinatingTransactions()

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the coordinating transactions metric
	var coordinatingMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_coordinating_txns" {
			coordinatingMetric = mf
			break
		}
	}

	assert.NotNil(t, coordinatingMetric, "coordinating_txns metric should exist")
	assert.Equal(t, coordinatingMetric.GetMetric()[0].GetGauge().GetValue(), float64(2))
}

func TestIncCoordinatingTransactions(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.IncCoordinatingTransactions()
	metrics.IncCoordinatingTransactions()

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the coordinating transactions metric
	var coordinatingMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_coordinating_txns" {
			coordinatingMetric = mf
			break
		}
	}

	assert.NotNil(t, coordinatingMetric, "coordinating_txns metric should exist")
	assert.Equal(t, coordinatingMetric.GetMetric()[0].GetGauge().GetValue(), float64(2))
}

func TestObserveSequencerTXStateChange(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	// Observe different states with different durations
	metrics.ObserveSequencerTXStateChange("accepted", 50*time.Millisecond)
	metrics.ObserveSequencerTXStateChange("assembled", 100*time.Millisecond)
	metrics.ObserveSequencerTXStateChange("accepted", 75*time.Millisecond)
	metrics.ObserveSequencerTXStateChange("endorsed", 200*time.Millisecond)

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the sequencer stage metric
	var stageMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_sequencer_stage" {
			stageMetric = mf
			break
		}
	}

	assert.NotNil(t, stageMetric, "sequencer_stage metric should exist")
	assert.Equal(t, dto.MetricType_HISTOGRAM, stageMetric.GetType(), "sequencer_stage should be a histogram")

	// Verify observations were recorded for different states
	metricsFound := 0
	for _, metric := range stageMetric.GetMetric() {
		labels := metric.GetLabel()
		if len(labels) > 0 {
			stage := labels[0].GetValue()
			histogram := metric.GetHistogram()
			if histogram != nil {
				sampleCount := histogram.GetSampleCount()
				assert.Greater(t, sampleCount, uint64(0), "Histogram should have observations for stage: %s", stage)
				metricsFound++
			}
		}
	}

	// Should have metrics for at least the states we observed
	assert.GreaterOrEqual(t, metricsFound, 2, "Should have metrics for multiple states")

	// Verify specific observations - check that "accepted" state has 2 observations
	var acceptedMetric *dto.Metric
	for _, metric := range stageMetric.GetMetric() {
		labels := metric.GetLabel()
		if len(labels) > 0 && labels[0].GetValue() == "accepted" {
			acceptedMetric = metric
			break
		}
	}
	assert.NotNil(t, acceptedMetric, "Should have metric for 'accepted' state")
	assert.Equal(t, uint64(2), acceptedMetric.GetHistogram().GetSampleCount(), "Should have 2 observations for 'accepted' state")
}

func TestSetActiveSequencers(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.SetActiveSequencers(5)
	metrics.SetActiveSequencers(10)
	metrics.SetActiveSequencers(3)

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the active sequencers metric
	var sequencersMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_active_sequencers" {
			sequencersMetric = mf
			break
		}
	}

	assert.NotNil(t, sequencersMetric, "active_sequencers metric should exist")
	assert.Equal(t, sequencersMetric.GetMetric()[0].GetGauge().GetValue(), float64(3))
}

