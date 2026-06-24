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

import "fmt"

// OriginatorState is the state of the originator top-level state machine.
type OriginatorState int

const (
	OriginatorState_Initial OriginatorState = iota
	OriginatorState_Idle
	OriginatorState_Observing
	OriginatorState_Sending
)

func (s OriginatorState) String() string {
	switch s {
	case OriginatorState_Initial:
		return "Initial"
	case OriginatorState_Idle:
		return "Idle"
	case OriginatorState_Observing:
		return "Observing"
	case OriginatorState_Sending:
		return "Sending"
	}
	return fmt.Sprintf("Unknown(%d)", s)
}

// CoordinatorState is the state of the coordinator top-level state machine.
type CoordinatorState int

const (
	CoordinatorState_Initial CoordinatorState = iota
	CoordinatorState_Idle
	CoordinatorState_Observing
	CoordinatorState_Elect
	CoordinatorState_Prepared
	CoordinatorState_Active
	CoordinatorState_Active_Flush
	CoordinatorState_Closing_Flush
	CoordinatorState_Closing
)

func (s CoordinatorState) String() string {
	switch s {
	case CoordinatorState_Initial:
		return "Initial"
	case CoordinatorState_Idle:
		return "Idle"
	case CoordinatorState_Observing:
		return "Observing"
	case CoordinatorState_Elect:
		return "Elect"
	case CoordinatorState_Prepared:
		return "Prepared"
	case CoordinatorState_Active:
		return "Active"
	case CoordinatorState_Active_Flush:
		return "Active_Flush"
	case CoordinatorState_Closing_Flush:
		return "Closing_Flush"
	case CoordinatorState_Closing:
		return "Closing"
	}
	return fmt.Sprintf("Unknown(%d)", s)
}

// OriginatorTransactionState is the state of the originator transaction state machine.
type OriginatorTransactionState int

const (
	OriginatorTransactionState_Initial OriginatorTransactionState = iota
	OriginatorTransactionState_Pending
	OriginatorTransactionState_Delegated
	OriginatorTransactionState_Assembling
	OriginatorTransactionState_Endorsement_Gathering
	OriginatorTransactionState_Prepared
	OriginatorTransactionState_Dispatched
	OriginatorTransactionState_Sequenced
	OriginatorTransactionState_Submitted
	OriginatorTransactionState_Confirmed
	OriginatorTransactionState_Reverted
	OriginatorTransactionState_Parked
	OriginatorTransactionState_Final
)

func (s OriginatorTransactionState) String() string {
	switch s {
	case OriginatorTransactionState_Initial:
		return "Initial"
	case OriginatorTransactionState_Pending:
		return "Pending"
	case OriginatorTransactionState_Delegated:
		return "Delegated"
	case OriginatorTransactionState_Assembling:
		return "Assembling"
	case OriginatorTransactionState_Endorsement_Gathering:
		return "Endorsement_Gathering"
	case OriginatorTransactionState_Prepared:
		return "Prepared"
	case OriginatorTransactionState_Dispatched:
		return "Dispatched"
	case OriginatorTransactionState_Sequenced:
		return "Sequenced"
	case OriginatorTransactionState_Submitted:
		return "Submitted"
	case OriginatorTransactionState_Confirmed:
		return "Confirmed"
	case OriginatorTransactionState_Reverted:
		return "Reverted"
	case OriginatorTransactionState_Parked:
		return "Parked"
	case OriginatorTransactionState_Final:
		return "Final"
	}
	return fmt.Sprintf("Unknown(%d)", s)
}

// CoordinatorTransactionState is the state of the coordinator transaction state machine.
type CoordinatorTransactionState int

const (
	CoordinatorTransactionState_Initial CoordinatorTransactionState = iota
	CoordinatorTransactionState_Pooled
	CoordinatorTransactionState_PreAssembly_Blocked
	CoordinatorTransactionState_Assembling
	CoordinatorTransactionState_Reverted
	CoordinatorTransactionState_Endorsement_Gathering
	CoordinatorTransactionState_Blocked
	CoordinatorTransactionState_Confirming_Dispatchable
	CoordinatorTransactionState_Ready_For_Dispatch
	CoordinatorTransactionState_Dispatched
	CoordinatorTransactionState_Confirmed
	CoordinatorTransactionState_Final
	CoordinatorTransactionState_Evicted
)

func (s CoordinatorTransactionState) String() string {
	switch s {
	case CoordinatorTransactionState_Initial:
		return "Initial"
	case CoordinatorTransactionState_Pooled:
		return "Pooled"
	case CoordinatorTransactionState_PreAssembly_Blocked:
		return "PreAssembly_Blocked"
	case CoordinatorTransactionState_Assembling:
		return "Assembling"
	case CoordinatorTransactionState_Reverted:
		return "Reverted"
	case CoordinatorTransactionState_Endorsement_Gathering:
		return "Endorsement_Gathering"
	case CoordinatorTransactionState_Blocked:
		return "Blocked"
	case CoordinatorTransactionState_Confirming_Dispatchable:
		return "Confirming_Dispatchable"
	case CoordinatorTransactionState_Ready_For_Dispatch:
		return "Ready_For_Dispatch"
	case CoordinatorTransactionState_Dispatched:
		return "Dispatched"
	case CoordinatorTransactionState_Confirmed:
		return "Confirmed"
	case CoordinatorTransactionState_Final:
		return "Final"
	case CoordinatorTransactionState_Evicted:
		return "Evicted"
	}
	return fmt.Sprintf("Unknown(%d)", s)
}
