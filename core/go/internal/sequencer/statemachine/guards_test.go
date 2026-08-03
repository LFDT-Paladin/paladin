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
	"testing"

	"github.com/stretchr/testify/assert"
)

type guardTestEntity struct {
	value int
}

func trueGuard(ctx context.Context, e *guardTestEntity) bool {
	return true
}

func falseGuard(ctx context.Context, e *guardTestEntity) bool {
	return false
}

func valueGreaterThan5(ctx context.Context, e *guardTestEntity) bool {
	return e.value > 5
}

func valueLessThan10(ctx context.Context, e *guardTestEntity) bool {
	return e.value < 10
}

func TestGuardNot(t *testing.T) {
	ctx := context.Background()
	entity := &guardTestEntity{}

	notTrue := GuardNot(trueGuard)
	notFalse := GuardNot(falseGuard)

	assert.False(t, notTrue(ctx, entity))
	assert.True(t, notFalse(ctx, entity))
}

func TestGuardAnd(t *testing.T) {
	ctx := context.Background()
	entity := &guardTestEntity{value: 7}

	// All true
	andAllTrue := GuardAnd(trueGuard, trueGuard)
	assert.True(t, andAllTrue(ctx, entity))

	// All false
	andAllFalse := GuardAnd(falseGuard, falseGuard)
	assert.False(t, andAllFalse(ctx, entity))

	// Mixed
	andMixed := GuardAnd(trueGuard, falseGuard)
	assert.False(t, andMixed(ctx, entity))

	// Empty (vacuous truth)
	andEmpty := GuardAnd[*guardTestEntity]()
	assert.True(t, andEmpty(ctx, entity))

	// Single
	andSingle := GuardAnd(trueGuard)
	assert.True(t, andSingle(ctx, entity))

	// Conditional - value is 7, so > 5 and < 10 are both true
	andConditional := GuardAnd(valueGreaterThan5, valueLessThan10)
	assert.True(t, andConditional(ctx, entity))

	// Conditional failing - value is 7, so > 5 is true but we negate it
	andConditionalFail := GuardAnd(GuardNot(valueGreaterThan5), valueLessThan10)
	assert.False(t, andConditionalFail(ctx, entity))
}

func TestGuardOr(t *testing.T) {
	ctx := context.Background()
	entity := &guardTestEntity{value: 7}

	// All true
	orAllTrue := GuardOr(trueGuard, trueGuard)
	assert.True(t, orAllTrue(ctx, entity))

	// All false
	orAllFalse := GuardOr(falseGuard, falseGuard)
	assert.False(t, orAllFalse(ctx, entity))

	// Mixed - at least one true
	orMixed := GuardOr(trueGuard, falseGuard)
	assert.True(t, orMixed(ctx, entity))

	orMixed2 := GuardOr(falseGuard, trueGuard)
	assert.True(t, orMixed2(ctx, entity))

	// Empty (vacuous false)
	orEmpty := GuardOr[*guardTestEntity]()
	assert.False(t, orEmpty(ctx, entity))

	// Single
	orSingle := GuardOr(falseGuard)
	assert.False(t, orSingle(ctx, entity))

	orSingleTrue := GuardOr(trueGuard)
	assert.True(t, orSingleTrue(ctx, entity))
}

func TestComplexGuardComposition(t *testing.T) {
	ctx := context.Background()

	// Complex: (value > 5 AND value < 10) OR (value == 0)
	isZero := func(ctx context.Context, e *guardTestEntity) bool {
		return e.value == 0
	}

	complexGuard := GuardOr(
		GuardAnd(valueGreaterThan5, valueLessThan10),
		isZero,
	)

	// value = 7: (7 > 5 AND 7 < 10) = true
	entity1 := &guardTestEntity{value: 7}
	assert.True(t, complexGuard(ctx, entity1))

	// value = 0: (0 > 5 = false, short circuit) OR (0 == 0 = true)
	entity2 := &guardTestEntity{value: 0}
	assert.True(t, complexGuard(ctx, entity2))

	// value = 15: (15 > 5 = true AND 15 < 10 = false) OR (15 == 0 = false) = false
	entity3 := &guardTestEntity{value: 15}
	assert.False(t, complexGuard(ctx, entity3))

	// value = 3: (3 > 5 = false) OR (3 == 0 = false) = false
	entity4 := &guardTestEntity{value: 3}
	assert.False(t, complexGuard(ctx, entity4))
}

func TestGuardShortCircuit(t *testing.T) {
	ctx := context.Background()
	entity := &guardTestEntity{}

	// Track how many times each guard is called
	callCount := 0

	countingTrueGuard := func(ctx context.Context, e *guardTestEntity) bool {
		callCount++
		return true
	}

	countingFalseGuard := func(ctx context.Context, e *guardTestEntity) bool {
		callCount++
		return false
	}

	// And should short-circuit on first false
	callCount = 0
	andShortCircuit := GuardAnd(countingFalseGuard, countingTrueGuard)
	assert.False(t, andShortCircuit(ctx, entity))
	assert.Equal(t, 1, callCount) // Should only call the first guard

	// Or should short-circuit on first true
	callCount = 0
	orShortCircuit := GuardOr(countingTrueGuard, countingFalseGuard)
	assert.True(t, orShortCircuit(ctx, entity))
	assert.Equal(t, 1, callCount) // Should only call the first guard
}
