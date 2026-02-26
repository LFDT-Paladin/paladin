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
package transaction

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func Test_clearTimeoutSchedules_BothNil(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.cancelRequestTimeoutSchedule = nil
	txn.cancelStateTimeoutSchedule = nil

	// Should not panic
	txn.clearTimeoutSchedules()

	assert.Nil(t, txn.cancelRequestTimeoutSchedule)
	assert.Nil(t, txn.cancelStateTimeoutSchedule)
}

func Test_clearTimeoutSchedules_BothSet(t *testing.T) {
	txn, _ := newTransactionForUnitTesting(t, nil)
	called1 := false
	called2 := false
	txn.cancelRequestTimeoutSchedule = func() {
		called1 = true
	}
	txn.cancelStateTimeoutSchedule = func() {
		called2 = true
	}

	txn.clearTimeoutSchedules()

	assert.True(t, called1)
	assert.True(t, called2)
	assert.Nil(t, txn.cancelRequestTimeoutSchedule)
	assert.Nil(t, txn.cancelStateTimeoutSchedule)
}

func Test_stateTimeoutExceeded_NoStartTime(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)
	txn.stateEntryTime = nil
	pendingRequest := common.NewIdempotentRequest(ctx, txn.clock, txn.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return nil
	})

	assert.False(t, txn.stateTimeoutExceeded(ctx, pendingRequest, "test-state"))
}
