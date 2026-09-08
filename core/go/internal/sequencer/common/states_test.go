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

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOriginatorState_String(t *testing.T) {
	assert.Equal(t, "Initial", OriginatorState_Initial.String())
	assert.Equal(t, "Idle", OriginatorState_Idle.String())
	assert.Equal(t, "Observing", OriginatorState_Observing.String())
	assert.Equal(t, "Sending", OriginatorState_Sending.String())
	assert.Equal(t, "Unknown(99)", OriginatorState(99).String())
}

func TestCoordinatorState_String(t *testing.T) {
	assert.Equal(t, "Initial", CoordinatorState_Initial.String())
	assert.Equal(t, "Idle", CoordinatorState_Idle.String())
	assert.Equal(t, "Observing", CoordinatorState_Observing.String())
	assert.Equal(t, "Elect", CoordinatorState_Elect.String())
	assert.Equal(t, "Prepared", CoordinatorState_Prepared.String())
	assert.Equal(t, "Active", CoordinatorState_Active.String())
	assert.Equal(t, "Active_Flush", CoordinatorState_Active_Flush.String())
	assert.Equal(t, "Closing_Flush", CoordinatorState_Closing_Flush.String())
	assert.Equal(t, "Closing", CoordinatorState_Closing.String())
	assert.Equal(t, "Unknown(99)", CoordinatorState(99).String())
}

func TestOriginatorTransactionState_String(t *testing.T) {
	assert.Equal(t, "Initial", OriginatorTransactionState_Initial.String())
	assert.Equal(t, "Resolving", OriginatorTransactionState_Resolving.String())
	assert.Equal(t, "Pending", OriginatorTransactionState_Pending.String())
	assert.Equal(t, "Delegated", OriginatorTransactionState_Delegated.String())
	assert.Equal(t, "Assembling", OriginatorTransactionState_Assembling.String())
	assert.Equal(t, "Signing", OriginatorTransactionState_Signing.String())
	assert.Equal(t, "Endorsement_Gathering", OriginatorTransactionState_Endorsement_Gathering.String())
	assert.Equal(t, "Prepared", OriginatorTransactionState_Prepared.String())
	assert.Equal(t, "Dispatched", OriginatorTransactionState_Dispatched.String())
	assert.Equal(t, "Sequenced", OriginatorTransactionState_Sequenced.String())
	assert.Equal(t, "Submitted", OriginatorTransactionState_Submitted.String())
	assert.Equal(t, "Confirmed", OriginatorTransactionState_Confirmed.String())
	assert.Equal(t, "Reverted", OriginatorTransactionState_Reverted.String())
	assert.Equal(t, "Parked", OriginatorTransactionState_Parked.String())
	assert.Equal(t, "Final", OriginatorTransactionState_Final.String())
	assert.Equal(t, "Unknown(99)", OriginatorTransactionState(99).String())
}

func TestCoordinatorTransactionState_String(t *testing.T) {
	assert.Equal(t, "Initial", CoordinatorTransactionState_Initial.String())
	assert.Equal(t, "Pooled", CoordinatorTransactionState_Pooled.String())
	assert.Equal(t, "PreAssembly_Blocked", CoordinatorTransactionState_PreAssembly_Blocked.String())
	assert.Equal(t, "Assembling", CoordinatorTransactionState_Assembling.String())
	assert.Equal(t, "Signing", CoordinatorTransactionState_Signing.String())
	assert.Equal(t, "Reverted", CoordinatorTransactionState_Reverted.String())
	assert.Equal(t, "Endorsement_Gathering", CoordinatorTransactionState_Endorsement_Gathering.String())
	assert.Equal(t, "Blocked", CoordinatorTransactionState_Blocked.String())
	assert.Equal(t, "Confirming_Dispatchable", CoordinatorTransactionState_Confirming_Dispatchable.String())
	assert.Equal(t, "Ready_For_Dispatch", CoordinatorTransactionState_Ready_For_Dispatch.String())
	assert.Equal(t, "Dispatched", CoordinatorTransactionState_Dispatched.String())
	assert.Equal(t, "Confirmed", CoordinatorTransactionState_Confirmed.String())
	assert.Equal(t, "Final", CoordinatorTransactionState_Final.String())
	assert.Equal(t, "Evicted", CoordinatorTransactionState_Evicted.String())
	assert.Equal(t, "Unknown(99)", CoordinatorTransactionState(99).String())
}
