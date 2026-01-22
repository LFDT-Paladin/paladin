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

func TestNot(t *testing.T) {
	ctx := context.Background()
	entity := &guardTestEntity{}

	notTrue := Not(trueGuard)
	notFalse := Not(falseGuard)

	assert.False(t, notTrue(ctx, entity))
	assert.True(t, notFalse(ctx, entity))
}

func TestAnd(t *testing.T) {
	ctx := context.Background()
	entity := &guardTestEntity{value: 7}

	// All true
	andAllTrue := And(trueGuard, trueGuard)
	assert.True(t, andAllTrue(ctx, entity))

	// All false
	andAllFalse := And(falseGuard, falseGuard)
	assert.False(t, andAllFalse(ctx, entity))

	// Mixed
	andMixed := And(trueGuard, falseGuard)
	assert.False(t, andMixed(ctx, entity))

	// Empty (vacuous truth)
	andEmpty := And[*guardTestEntity]()
	assert.True(t, andEmpty(ctx, entity))

	// Single
	andSingle := And(trueGuard)
	assert.True(t, andSingle(ctx, entity))

	// Conditional - value is 7, so > 5 and < 10 are both true
	andConditional := And(valueGreaterThan5, valueLessThan10)
	assert.True(t, andConditional(ctx, entity))

	// Conditional failing - value is 7, so > 5 is true but we negate it
	andConditionalFail := And(Not(valueGreaterThan5), valueLessThan10)
	assert.False(t, andConditionalFail(ctx, entity))
}

func TestOr(t *testing.T) {
	ctx := context.Background()
	entity := &guardTestEntity{value: 7}

	// All true
	orAllTrue := Or(trueGuard, trueGuard)
	assert.True(t, orAllTrue(ctx, entity))

	// All false
	orAllFalse := Or(falseGuard, falseGuard)
	assert.False(t, orAllFalse(ctx, entity))

	// Mixed - at least one true
	orMixed := Or(trueGuard, falseGuard)
	assert.True(t, orMixed(ctx, entity))

	orMixed2 := Or(falseGuard, trueGuard)
	assert.True(t, orMixed2(ctx, entity))

	// Empty (vacuous false)
	orEmpty := Or[*guardTestEntity]()
	assert.False(t, orEmpty(ctx, entity))

	// Single
	orSingle := Or(falseGuard)
	assert.False(t, orSingle(ctx, entity))

	orSingleTrue := Or(trueGuard)
	assert.True(t, orSingleTrue(ctx, entity))
}

func TestAlways(t *testing.T) {
	ctx := context.Background()
	entity := &guardTestEntity{value: 0}

	always := Always[*guardTestEntity]()
	assert.True(t, always(ctx, entity))

	// Even with different entity state
	entity.value = 100
	assert.True(t, always(ctx, entity))
}

func TestNever(t *testing.T) {
	ctx := context.Background()
	entity := &guardTestEntity{value: 0}

	never := Never[*guardTestEntity]()
	assert.False(t, never(ctx, entity))

	// Even with different entity state
	entity.value = 100
	assert.False(t, never(ctx, entity))
}

func TestComplexGuardComposition(t *testing.T) {
	ctx := context.Background()

	// Complex: (value > 5 AND value < 10) OR (value == 0)
	isZero := func(ctx context.Context, e *guardTestEntity) bool {
		return e.value == 0
	}

	complexGuard := Or(
		And(valueGreaterThan5, valueLessThan10),
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
	andShortCircuit := And(countingFalseGuard, countingTrueGuard)
	assert.False(t, andShortCircuit(ctx, entity))
	assert.Equal(t, 1, callCount) // Should only call the first guard

	// Or should short-circuit on first true
	callCount = 0
	orShortCircuit := Or(countingTrueGuard, countingFalseGuard)
	assert.True(t, orShortCircuit(ctx, entity))
	assert.Equal(t, 1, callCount) // Should only call the first guard
}
