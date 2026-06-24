// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package msgs

var (
	MsgCoordinatorTxStateInitial                = pdm("coordinator.transaction.State_Initial", "Transaction state machine has been created")
	MsgCoordinatorTxStatePooled                 = pdm("coordinator.transaction.State_Pooled", "The transaction is waiting in the pool to be selected and sent for assembly to the its originator")
	MsgCoordinatorTxStatePreAssemblyBlocked     = pdm("coordinator.transaction.State_PreAssembly_Blocked", "The transaction cannot yet be put in the pool to be selected for assembly because a dependency must be assembled first")
	MsgCoordinatorTxStateAssembling             = pdm("coordinator.transaction.State_Assembling", "An assemble request has been sent to the originator and we are waiting for the response")
	MsgCoordinatorTxStateReverted               = pdm("coordinator.transaction.State_Reverted", "The transaction has been reverted, either at assembly time by the originator or on the base ledger")
	MsgCoordinatorTxStateEndorsementGathering   = pdm("coordinator.transaction.State_Endorsement_Gathering", "The transaction has been successfully assembled and endorsement requests have been sent")
	MsgCoordinatorTxStateBlocked                = pdm("coordinator.transaction.State_Blocked", "All endorsements have been received but the transaction cannot proceed due to dependencies not being ready for dispatch")
	MsgCoordinatorTxStateConfirmingDispatchable = pdm("coordinator.transaction.State_Confirming_Dispatchable", "The transaction has been endorsed. Confirmation from the originator is required before the transaction can be dispatched. The originator may still request not to proceed at this point.")
	MsgCoordinatorTxStateReadyForDispatch       = pdm("coordinator.transaction.State_Ready_For_Dispatch", "Dispatch confirmation has been received from the originator and the transaction is waiting to be collected by the dispatch goroutine")
	MsgCoordinatorTxStateDispatched             = pdm("coordinator.transaction.State_Dispatched", "Collected by the dispatcher thread and submitted by the public TX manager to the base ledger")
	MsgCoordinatorTxStateConfirmed              = pdm("coordinator.transaction.State_Confirmed", "The transaction has been confirmed on the base ledger. It will remain in this state for a number heartbeat intervals before moving to State_Final to removed from memory.")
	MsgCoordinatorTxStateFinal                  = pdm("coordinator.transaction.State_Final", "The transaction will be removed from memory upon entry to this state")
	MsgCoordinatorTxStateEvicted                = pdm("coordinator.transaction.State_Evicted", "A problematic transaction is being evicted. Transactions are removed from memory upon entry to this this state. Distinct from State_Final because it might just be used for memory or in-flight slot management")
)
