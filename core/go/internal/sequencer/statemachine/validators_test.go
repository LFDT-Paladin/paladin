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

package statemachine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validatorTestEntity struct {
	value int
}

const validatorEventType common.EventType = 900001

type validatorTestEvent struct {
	common.BaseEvent
}

func (e *validatorTestEvent) Type() common.EventType {
	return validatorEventType
}

func (e *validatorTestEvent) TypeString() string {
	return "validator_test_event"
}

func newValidatorTestEvent() *validatorTestEvent {
	return &validatorTestEvent{
		BaseEvent: common.BaseEvent{EventTime: time.Now()},
	}
}

func trueValidator(ctx context.Context, e *validatorTestEntity, event common.Event) (bool, error) {
	return true, nil
}

func falseValidator(ctx context.Context, e *validatorTestEntity, event common.Event) (bool, error) {
	return false, nil
}

func valueGreaterThan5Validator(ctx context.Context, e *validatorTestEntity, event common.Event) (bool, error) {
	return e.value > 5, nil
}

func valueLessThan10Validator(ctx context.Context, e *validatorTestEntity, event common.Event) (bool, error) {
	return e.value < 10, nil
}

func TestValidatorAnd(t *testing.T) {
	ctx := context.Background()
	entity := &validatorTestEntity{value: 7}
	event := newValidatorTestEvent()

	allTrue := ValidatorAnd(trueValidator, trueValidator)
	valid, err := allTrue(ctx, entity, event)
	require.NoError(t, err)
	assert.True(t, valid)

	allFalse := ValidatorAnd(falseValidator, falseValidator)
	valid, err = allFalse(ctx, entity, event)
	require.NoError(t, err)
	assert.False(t, valid)

	mixed := ValidatorAnd(trueValidator, falseValidator)
	valid, err = mixed(ctx, entity, event)
	require.NoError(t, err)
	assert.False(t, valid)

	empty := ValidatorAnd[*validatorTestEntity]()
	valid, err = empty(ctx, entity, event)
	require.NoError(t, err)
	assert.True(t, valid)

	conditional := ValidatorAnd(valueGreaterThan5Validator, valueLessThan10Validator)
	valid, err = conditional(ctx, entity, event)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestValidatorAnd_Error(t *testing.T) {
	ctx := context.Background()
	entity := &validatorTestEntity{}
	event := newValidatorTestEvent()
	expectedErr := errors.New("validator failed")

	validator := ValidatorAnd(
		trueValidator,
		func(ctx context.Context, e *validatorTestEntity, event common.Event) (bool, error) {
			return false, expectedErr
		},
	)

	valid, err := validator(ctx, entity, event)
	assert.False(t, valid)
	assert.Equal(t, expectedErr, err)
}

func TestValidatorOr(t *testing.T) {
	ctx := context.Background()
	entity := &validatorTestEntity{value: 7}
	event := newValidatorTestEvent()

	allTrue := ValidatorOr(trueValidator, trueValidator)
	valid, err := allTrue(ctx, entity, event)
	require.NoError(t, err)
	assert.True(t, valid)

	allFalse := ValidatorOr(falseValidator, falseValidator)
	valid, err = allFalse(ctx, entity, event)
	require.NoError(t, err)
	assert.False(t, valid)

	mixed := ValidatorOr(trueValidator, falseValidator)
	valid, err = mixed(ctx, entity, event)
	require.NoError(t, err)
	assert.True(t, valid)

	empty := ValidatorOr[*validatorTestEntity]()
	valid, err = empty(ctx, entity, event)
	require.NoError(t, err)
	assert.False(t, valid)

	conditional := ValidatorOr(valueGreaterThan5Validator, valueLessThan10Validator)
	valid, err = conditional(ctx, entity, event)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestValidatorOr_Error(t *testing.T) {
	ctx := context.Background()
	entity := &validatorTestEntity{}
	event := newValidatorTestEvent()
	expectedErr := errors.New("validator failed")

	validator := ValidatorOr(
		falseValidator,
		func(ctx context.Context, e *validatorTestEntity, event common.Event) (bool, error) {
			return false, expectedErr
		},
	)

	valid, err := validator(ctx, entity, event)
	assert.False(t, valid)
	assert.Equal(t, expectedErr, err)
}

func TestValidatorNot(t *testing.T) {
	ctx := context.Background()
	entity := &validatorTestEntity{value: 7}
	event := newValidatorTestEvent()

	notTrue := ValidatorNot(trueValidator)
	valid, err := notTrue(ctx, entity, event)
	require.NoError(t, err)
	assert.False(t, valid)

	notFalse := ValidatorNot(falseValidator)
	valid, err = notFalse(ctx, entity, event)
	require.NoError(t, err)
	assert.True(t, valid)

	notConditional := ValidatorNot(valueGreaterThan5Validator)
	valid, err = notConditional(ctx, entity, event)
	require.NoError(t, err)
	assert.False(t, valid)

	entityBelow := &validatorTestEntity{value: 3}
	valid, err = notConditional(ctx, entityBelow, event)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestValidatorNot_Error(t *testing.T) {
	ctx := context.Background()
	entity := &validatorTestEntity{}
	event := newValidatorTestEvent()
	expectedErr := errors.New("validator failed")

	validator := ValidatorNot(
		func(ctx context.Context, e *validatorTestEntity, event common.Event) (bool, error) {
			return false, expectedErr
		},
	)

	valid, err := validator(ctx, entity, event)
	assert.False(t, valid)
	assert.Equal(t, expectedErr, err)
}

func TestValidatorShortCircuit(t *testing.T) {
	ctx := context.Background()
	entity := &validatorTestEntity{}
	event := newValidatorTestEvent()

	callCount := 0
	countingTrue := func(ctx context.Context, e *validatorTestEntity, event common.Event) (bool, error) {
		callCount++
		return true, nil
	}
	countingFalse := func(ctx context.Context, e *validatorTestEntity, event common.Event) (bool, error) {
		callCount++
		return false, nil
	}

	callCount = 0
	andShortCircuit := ValidatorAnd(countingFalse, countingTrue)
	valid, err := andShortCircuit(ctx, entity, event)
	require.NoError(t, err)
	assert.False(t, valid)
	assert.Equal(t, 1, callCount)

	callCount = 0
	orShortCircuit := ValidatorOr(countingTrue, countingFalse)
	valid, err = orShortCircuit(ctx, entity, event)
	require.NoError(t, err)
	assert.True(t, valid)
	assert.Equal(t, 1, callCount)
}
