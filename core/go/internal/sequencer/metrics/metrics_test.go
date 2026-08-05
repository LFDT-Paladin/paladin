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
	metrics.ObserveSequencerTXStateChange("coordinator", "accepted", 50*time.Millisecond)
	metrics.ObserveSequencerTXStateChange("coordinator", "assembled", 100*time.Millisecond)
	metrics.ObserveSequencerTXStateChange("coordinator", "accepted", 75*time.Millisecond)
	metrics.ObserveSequencerTXStateChange("coordinator", "endorsed", 200*time.Millisecond)

	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")

	// Find the sequencer stage metric
	var stageMetric *dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "distributed_sequencer_stage_duration_ms" {
			stageMetric = mf
			break
		}
	}

	stageLabel := func(metric *dto.Metric) string {
		for _, l := range metric.GetLabel() {
			if l.GetName() == "stage" {
				return l.GetValue()
			}
		}
		return ""
	}

	assert.NotNil(t, stageMetric, "stage_duration_ms metric should exist")
	assert.Equal(t, dto.MetricType_HISTOGRAM, stageMetric.GetType(), "stage_duration_ms should be a histogram")

	// Verify observations were recorded for different states
	metricsFound := 0
	for _, metric := range stageMetric.GetMetric() {
		stage := stageLabel(metric)
		if stage != "" {
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
		if stageLabel(metric) == "accepted" {
			acceptedMetric = metric
			break
		}
	}
	assert.NotNil(t, acceptedMetric, "Should have metric for 'accepted' state")
	assert.Equal(t, uint64(2), acceptedMetric.GetHistogram().GetSampleCount(), "Should have 2 observations for 'accepted' state")
}

func findMetricFamily(t *testing.T, registry *prometheus.Registry, name string) *dto.MetricFamily {
	metricFamilies, err := registry.Gather()
	assert.NoError(t, err, "Unexpected error gathering metrics")
	for _, mf := range metricFamilies {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

func labelValue(metric *dto.Metric, name string) string {
	for _, l := range metric.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}

func TestObserveDomainCall(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.ObserveDomainCall("noto", "assemble", 50*time.Millisecond)
	metrics.ObserveDomainCall("noto", "assemble", 75*time.Millisecond)

	mf := findMetricFamily(t, registry, "distributed_sequencer_domain_call_ms")
	assert.NotNil(t, mf, "domain_call_ms metric should exist")
	assert.Equal(t, dto.MetricType_HISTOGRAM, mf.GetType(), "domain_call_ms should be a histogram")

	var m *dto.Metric
	for _, metric := range mf.GetMetric() {
		if labelValue(metric, "domain") == "noto" && labelValue(metric, "method") == "assemble" {
			m = metric
			break
		}
	}
	assert.NotNil(t, m, "Should have a metric labelled domain=noto method=assemble")
	assert.Equal(t, uint64(2), m.GetHistogram().GetSampleCount(), "Should have 2 observations")
}

func TestObserveAssembleResponseApply(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.ObserveAssembleResponseApply(10 * time.Millisecond)
	metrics.ObserveAssembleResponseApply(20 * time.Millisecond)
	metrics.ObserveAssembleResponseApply(30 * time.Millisecond)

	mf := findMetricFamily(t, registry, "distributed_sequencer_assemble_response_apply_ms")
	assert.NotNil(t, mf, "assemble_response_apply_ms metric should exist")
	assert.Equal(t, dto.MetricType_HISTOGRAM, mf.GetType(), "assemble_response_apply_ms should be a histogram")
	assert.Equal(t, uint64(3), mf.GetMetric()[0].GetHistogram().GetSampleCount(), "Should have 3 observations")
}

func TestObserveDispatchQueueWait(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.ObserveDispatchQueueWait(5 * time.Millisecond)
	metrics.ObserveDispatchQueueWait(15 * time.Millisecond)

	mf := findMetricFamily(t, registry, "distributed_sequencer_dispatch_queue_wait_ms")
	assert.NotNil(t, mf, "dispatch_queue_wait_ms metric should exist")
	assert.Equal(t, dto.MetricType_HISTOGRAM, mf.GetType(), "dispatch_queue_wait_ms should be a histogram")
	assert.Equal(t, uint64(2), mf.GetMetric()[0].GetHistogram().GetSampleCount(), "Should have 2 observations")
}

func TestObserveDispatchInflightWait(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.ObserveDispatchInflightWait(2 * time.Millisecond)
	metrics.ObserveDispatchInflightWait(4 * time.Millisecond)
	metrics.ObserveDispatchInflightWait(6 * time.Millisecond)

	mf := findMetricFamily(t, registry, "distributed_sequencer_dispatch_inflight_wait_ms")
	assert.NotNil(t, mf, "dispatch_inflight_wait_ms metric should exist")
	assert.Equal(t, dto.MetricType_HISTOGRAM, mf.GetType(), "dispatch_inflight_wait_ms should be a histogram")
	assert.Equal(t, uint64(3), mf.GetMetric()[0].GetHistogram().GetSampleCount(), "Should have 3 observations")
}

func TestObserveEventProcessing(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.ObserveEventProcessing("coordinator", "SomeEvent", 5*time.Millisecond)
	metrics.ObserveEventProcessing("coordinator", "SomeEvent", 10*time.Millisecond)

	mf := findMetricFamily(t, registry, "distributed_sequencer_event_processing_ms")
	assert.NotNil(t, mf, "event_processing_ms metric should exist")
	assert.Equal(t, dto.MetricType_HISTOGRAM, mf.GetType(), "event_processing_ms should be a histogram")

	var m *dto.Metric
	for _, metric := range mf.GetMetric() {
		if labelValue(metric, "role") == "coordinator" && labelValue(metric, "event_type") == "SomeEvent" {
			m = metric
			break
		}
	}
	assert.NotNil(t, m, "Should have a metric labelled role=coordinator event_type=SomeEvent")
	assert.Equal(t, uint64(2), m.GetHistogram().GetSampleCount(), "Should have 2 observations")
}

func TestSetEventQueueDepth(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.SetEventQueueDepth("originator", "normal", 3)
	metrics.SetEventQueueDepth("originator", "normal", 7)

	mf := findMetricFamily(t, registry, "distributed_sequencer_event_queue_depth")
	assert.NotNil(t, mf, "event_queue_depth metric should exist")

	var m *dto.Metric
	for _, metric := range mf.GetMetric() {
		if labelValue(metric, "role") == "originator" && labelValue(metric, "priority") == "normal" {
			m = metric
			break
		}
	}
	assert.NotNil(t, m, "Should have a metric labelled role=originator priority=normal")
	assert.Equal(t, float64(7), m.GetGauge().GetValue(), "Gauge should reflect the last value set")
}

func TestSetPooledTxns(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.SetPooledTxns(4)
	metrics.SetPooledTxns(9)
	metrics.SetPooledTxns(2)

	mf := findMetricFamily(t, registry, "distributed_sequencer_pooled_txns")
	assert.NotNil(t, mf, "pooled_txns metric should exist")
	assert.Equal(t, float64(2), mf.GetMetric()[0].GetGauge().GetValue(), "Gauge should reflect the last value set")
}

func TestSetInflightDispatchedTxns(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.SetInflightDispatchedTxns(1)
	metrics.SetInflightDispatchedTxns(5)

	mf := findMetricFamily(t, registry, "distributed_sequencer_inflight_dispatched_txns")
	assert.NotNil(t, mf, "inflight_dispatched_txns metric should exist")
	assert.Equal(t, float64(5), mf.GetMetric()[0].GetGauge().GetValue(), "Gauge should reflect the last value set")
}

func TestObserveDispatchBatchSize(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	metrics.ObserveDispatchBatchSize("public", 3)
	metrics.ObserveDispatchBatchSize("public", 5)
	metrics.ObserveDispatchBatchSize("private", 1)

	mf := findMetricFamily(t, registry, "distributed_sequencer_dispatch_batch_size")
	assert.NotNil(t, mf, "dispatch_batch_size metric should exist")
	assert.Equal(t, dto.MetricType_HISTOGRAM, mf.GetType(), "dispatch_batch_size should be a histogram")

	var publicMetric *dto.Metric
	for _, metric := range mf.GetMetric() {
		if labelValue(metric, "kind") == "public" {
			publicMetric = metric
			break
		}
	}
	assert.NotNil(t, publicMetric, "Should have a metric labelled kind=public")
	assert.Equal(t, uint64(2), publicMetric.GetHistogram().GetSampleCount(), "Should have 2 observations for kind=public")
}

func TestEventLoopMetricsAdapter(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := InitMetrics(context.Background(), registry)
	assert.NotNil(t, metrics)

	// The adapter bakes in a fixed role and forwards to the underlying metrics.
	adapter := NewEventLoopMetrics(metrics, "coordinator")
	assert.NotNil(t, adapter)

	adapter.ObserveEventProcessing("SomeEvent", 5*time.Millisecond)
	adapter.SetEventQueueDepth("priority", 6)

	// Event processing is recorded with the baked-in role.
	processing := findMetricFamily(t, registry, "distributed_sequencer_event_processing_ms")
	assert.NotNil(t, processing, "event_processing_ms metric should exist")
	var processingMetric *dto.Metric
	for _, metric := range processing.GetMetric() {
		if labelValue(metric, "role") == "coordinator" && labelValue(metric, "event_type") == "SomeEvent" {
			processingMetric = metric
			break
		}
	}
	assert.NotNil(t, processingMetric, "Adapter should record processing under role=coordinator")
	assert.Equal(t, uint64(1), processingMetric.GetHistogram().GetSampleCount(), "Should have 1 observation")

	// Queue depth is recorded with the baked-in role.
	depth := findMetricFamily(t, registry, "distributed_sequencer_event_queue_depth")
	assert.NotNil(t, depth, "event_queue_depth metric should exist")
	var depthMetric *dto.Metric
	for _, metric := range depth.GetMetric() {
		if labelValue(metric, "role") == "coordinator" && labelValue(metric, "priority") == "priority" {
			depthMetric = metric
			break
		}
	}
	assert.NotNil(t, depthMetric, "Adapter should record queue depth under role=coordinator")
	assert.Equal(t, float64(6), depthMetric.GetGauge().GetValue(), "Gauge should reflect the value set via the adapter")
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
