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
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_clearTimeoutSchedules_BothNil(t *testing.T) {
	txn, _ := NewTransactionBuilderForTesting(t, State_Initial).Build()

	// Should not panic
	txn.clearTimeoutSchedules()

	assert.Nil(t, txn.cancelRequestTimeoutSchedule)
	assert.Nil(t, txn.cancelStateTimeoutSchedule)
}

func Test_clearTimeoutSchedules_BothSet(t *testing.T) {
	called1 := false
	called2 := false
	txn, _ := NewTransactionBuilderForTesting(t, State_Assembling).
		CancelRequestTimeoutSchedule(func() {
			called1 = true
		}).
		CancelStateTimeoutSchedule(func() {
			called2 = true
		}).
		Build()

	txn.clearTimeoutSchedules()

	assert.True(t, called1)
	assert.True(t, called2)
	assert.Nil(t, txn.cancelRequestTimeoutSchedule)
	assert.Nil(t, txn.cancelStateTimeoutSchedule)
}
