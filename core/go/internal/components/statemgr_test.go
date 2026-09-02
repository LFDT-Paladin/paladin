/*
 * Copyright © 2026 Kaleido, Inc.
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

package components

import (
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/filters"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/stretchr/testify/assert"
)

func TestStateWithLabels_ValueSet(t *testing.T) {
	// Create a PassthroughValueSet with some test values
	labelValues := filters.PassthroughValueSet{
		".id":      "0x1234567890abcdef",
		".created": int64(1234567890),
		"field1":   "value1",
	}

	// Create a StateWithLabels instance
	state := &StateWithLabels{
		State:       &pldapi.State{},
		LabelValues: labelValues,
	}

	// Call ValueSet() and verify it returns the same LabelValues
	result := state.ValueSet()
	assert.Equal(t, labelValues, result)
}

func TestStateWithLabels_ValueSet_Empty(t *testing.T) {
	// Test with an empty ValueSet
	labelValues := filters.PassthroughValueSet{}

	state := &StateWithLabels{
		State:       &pldapi.State{},
		LabelValues: labelValues,
	}

	result := state.ValueSet()
	assert.Equal(t, labelValues, result)
}

func TestStateWithLabels_ValueSet_Nil(t *testing.T) {
	// Test with nil LabelValues
	state := &StateWithLabels{
		State:       &pldapi.State{},
		LabelValues: nil,
	}

	result := state.ValueSet()
	assert.Nil(t, result)
}

func TestStateWithLabels_ProtoLabels_ConvertsAllLabelTypes(t *testing.T) {
	state := &StateWithLabels{State: &pldapi.State{
		Labels:      []*pldapi.StateLabel{{Label: "owner", Value: "0xfeed"}},
		Int64Labels: []*pldapi.StateInt64Label{{Label: "amount", Value: 42}, {Label: "flag", Value: 1}},
	}}

	pl := state.ProtoLabels()
	assert.Len(t, pl.GetLabels(), 1)
	assert.Equal(t, "owner", pl.GetLabels()[0].GetLabel())
	assert.Equal(t, "0xfeed", pl.GetLabels()[0].GetValue())
	byName := map[string]int64{}
	for _, l := range pl.GetInt64Labels() {
		byName[l.GetLabel()] = l.GetValue()
	}
	assert.Equal(t, map[string]int64{"amount": 42, "flag": 1}, byName)
}

func TestStateWithLabels_ProtoLabels_Empty(t *testing.T) {
	state := &StateWithLabels{State: &pldapi.State{}}
	pl := state.ProtoLabels()
	assert.NotNil(t, pl)
	assert.Empty(t, pl.GetLabels())
	assert.Empty(t, pl.GetInt64Labels())
}

func TestStateWithLabels_ValueSet_WithResolvingValueSet(t *testing.T) {
	// Test with a different ValueSet implementation (ResolvingValueSet)
	labelValues := filters.ResolvingValueSet{
		"field1": []byte(`"value1"`),
		"field2": []byte(`"value2"`),
	}

	state := &StateWithLabels{
		State:       &pldapi.State{},
		LabelValues: labelValues,
	}

	result := state.ValueSet()
	assert.Equal(t, labelValues, result)
}
