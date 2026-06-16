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

package coordinator

import (
	"context"
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
)

// State is a local alias for common.CoordinatorState
type State = common.CoordinatorState

// EventType is an alias for common.EventType
type EventType = common.EventType

// Note: inline comments on State_* constants are used in auto-generated documentation.
// Keep them accurate and human-readable - see scripts/generate_state_machine_docs.py
const (

	State_Initial       = common.CoordinatorState_Initial       // Coordinator state machine created
	State_Idle          = common.CoordinatorState_Idle          // Not actively coordinating and not aware of any other active coordinators
	State_Observing     = common.CoordinatorState_Observing     // Not actively coordinating but aware of another node actively coordinating
	State_Elect         = common.CoordinatorState_Elect         // Has sent a handover request to an active coordinator and is waiting for that node to stop coordinating
	State_Prepared      = common.CoordinatorState_Prepared      // Has seen the previous active coordinator begin to flush and is waiting for the flush to complete
	State_Active        = common.CoordinatorState_Active        // Actively coordinating transactions for this domain instance
	State_Active_Flush  = common.CoordinatorState_Active_Flush  // Draining dispatched transactions while still the active coordinator (key-rotation)
	State_Closing_Flush = common.CoordinatorState_Closing_Flush // Draining dispatched transactions after stepping down (preemption)
	State_Closing       = common.CoordinatorState_Closing       // Has flushed and is continuing to send closing status for configured number of heartbeats
)

const (
	Event_CoordinatorCreated EventType = iota + 200
	Event_TransactionsDelegated
	Event_RequestTimeoutInterval
	Event_StateTimeoutInterval
	Event_HandoverRequest            // pushed by transport_client when a CoordinatorHandoverRequest message is received from a higher-priority node
	Event_RestartDispatchLoop        // queued internally after an in-place key rotation so the loop restarts after TransactionStateTransitionEvents are processed
	Event_EndorsementRequestReceived // pushed by transport_client when an EndorsementRequest message arrives for this coordinator
	Event_EpochBoundaryReached       // queued internally by getAndRefreshBlockHeight when the effective block height advances to a new epoch
)

// Type aliases for the generic statemachine types, specialized for coordinator
type (
	Action           = statemachine.Action[*coordinator]
	Guard            = statemachine.Guard[*coordinator]
	Validator        = statemachine.Validator[*coordinator]
	ActionRule       = statemachine.ActionRule[*coordinator]
	Transition       = statemachine.Transition[State, *coordinator]
	EventHandler     = statemachine.EventHandler[State, *coordinator]
	EventHandlers    = statemachine.EventHandlers[State, *coordinator]
	StateDefinition  = statemachine.StateDefinition[State, *coordinator]
	StateDefinitions = statemachine.StateDefinitions[State, *coordinator]
)

var stateDefinitionsMap = StateDefinitions{
	State_Initial: {
		Events: map[EventType]EventHandlers{
			Event_CoordinatorCreated: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions:     []ActionRule{{Action: action_CalculateCoordinatorPriorities}},
					Transitions: []Transition{{To: State_Idle}},
				}},
			},
		},
	},
	State_Idle: {
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_AddEndorsersFromSnapshot,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsHeartbeatSenderLive,
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinator},
					},
					Transitions: []Transition{{
						To: State_Observing,
					}},
				}},
			},
			Event_EndorsementRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Actions: []ActionRule{{
						Action: action_AddEndorsementRequestSenderToEndorserCandidates,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsEndorsementBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectEndorsementBlockHeight}},
				}, {
					Validator: validator_IsPrivateStateDataPendingForEndorsement,
					Actions:   []ActionRule{{Action: action_RejectEndorsementPrivateStateDataPending}},
				}, {
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
					),
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinatorFromEndorsementRequest},
						{Action: action_HandleEndorsementRequest},
					},
					Transitions: []Transition{{To: State_Observing}},
				}},
			},
			Event_TransactionsDelegated: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Validator: validator_IsDelegationBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectDelegationRequestBlockHeight}},
				}, {
					// Any node in Idle accepts a delegation and becomes the active coordinator.
					// A higher-priority node that later announces itself will trigger preemption from Active.
					Validator:   statemachine.ValidatorNot(validator_IsDelegationBlockHeightToleranceExceeded),
					Actions:     []ActionRule{{Action: action_ProcessDelegatedTransactions}},
					Transitions: []Transition{{To: State_Active}},
				}},
			},
		},
	},
	State_Observing: {
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_AddEndorsersFromSnapshot,
						If:     guard_IsCoordinatorEndorserSelectionMode}},
				}, {
					Validator: validator_IsHeartbeatSenderLive,
					Actions: []ActionRule{
						{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
						{Action: action_UpdateActiveCoordinator},
					},
				}},
			},
			Event_EndorsementRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Actions: []ActionRule{{
						Action: action_AddEndorsementRequestSenderToEndorserCandidates,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsEndorsementBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectEndorsementBlockHeight}},
				}, {
					Validator: validator_IsPrivateStateDataPendingForEndorsement,
					Actions:   []ActionRule{{Action: action_RejectEndorsementPrivateStateDataPending}},
				}, {
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
					),
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinatorFromEndorsementRequest},
						{Action: action_HandleEndorsementRequest},
					},
				}},
			},
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateOriginatorActivity},
						{Action: action_IncrementHeartbeatIntervalCounts},
					},
					Transitions: []Transition{{
						To: State_Idle,
						If: guard_InactiveGracePeriodExceeded,
					}},
				}},
			},
			Event_TransactionsDelegated: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Validator: validator_IsDelegationBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectDelegationRequestBlockHeight}},
				}, {
					Validator: statemachine.ValidatorNot(validator_IsDelegationBlockHeightToleranceExceeded),
					Actions: []ActionRule{
						{
							// This node is higher-priority than the current active coordinator — initiate a handover.
							If:     guard_IsHigherPriorityThanCurrentActive,
							Action: action_ProcessDelegatedTransactions,
						},
						{
							// This node is lower-priority — reject and include the current active coordinator's identity.
							If:     statemachine.GuardNot(guard_IsHigherPriorityThanCurrentActive),
							Action: action_RejectDelegationRequest,
						},
					},
					Transitions: []Transition{{
						To: State_Elect,
						If: guard_IsHigherPriorityThanCurrentActive,
					}},
				}},
			},
		},
	},
	State_Elect: {
		OnTransitionTo: []ActionRule{
			{Action: action_ScheduleStateTimeout},
			{Action: action_SendHandoverRequest},
		},
		Events: map[EventType]EventHandlers{
			Event_RequestTimeoutInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_NudgeHandoverRequest}},
				}},
			},
			Event_StateTimeoutInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					// The active coordinator has not moved to flush; become active directly.
					// This may result in state contention but the protocol allows for this while
					// the system eventually normalises.
					Transitions: []Transition{{
						To:      State_Active,
						Actions: []ActionRule{{Action: action_ClearTimeoutSchedules}},
					}},
				}},
			},
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateOriginatorActivity},
						{Action: action_PropagateHeartbeatIntervalToTransactions},
						{Action: action_SendHeartbeat},
					},
				}},
			},
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_AddEndorsersFromSnapshot,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					// We're not going to take over if see an active heartbeat from a higher priority coordinator.
					// Clean up any delegations and choose an appropriate state to move back to.
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatFromHigherPriorityCoordinator,
						validator_IsHeartbeatSenderLive,
					),
					Actions: []ActionRule{
						{Action: action_CleanUpTransactionsNotYetDispatched},
						{Action: action_UpdateActiveCoordinator},
						{Action: action_ClearTimeoutSchedules},
					},
					Transitions: []Transition{{
						// Move to observing if we don't have any transactions in flight.
						To: State_Observing,
						If: statemachine.GuardNot(guard_HasTransactionsInflight),
					}, {
						// Flush if we still have unconfirmed transactions from when we were last active.
						To: State_Closing_Flush,
						If: statemachine.GuardAnd(guard_HasTransactionsInflight, guard_HasUnconfirmedDispatchedTransactions),
					}, {
						// Otherwise move back to closing while we wait for our previously confirmed
						// transactions to be cleared from memory.
						To: State_Closing,
						If: statemachine.GuardAnd(guard_HasTransactionsInflight, statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions)),
					}},
				}, {
					// The active coordinator has started flushing in response to our handover request.
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatFromCurrentActiveCoordinator,
						validator_HeartBeatState(State_Closing_Flush),
					),
					Actions:     []ActionRule{{Action: action_ClearTimeoutSchedules}},
					Transitions: []Transition{{To: State_Prepared}},
				}, {
					// The active coordinator has closed in response to our handover request.
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatFromCurrentActiveCoordinator,
						validator_HeartBeatState(State_Closing),
					),
					Actions: []ActionRule{
						{Action: action_ClearTimeoutSchedules},
						{Action: action_ImportStatesAndLocks},
					},
					Transitions: []Transition{{To: State_Active}},
				}},
			},
			Event_HandoverRequest: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsHandoverRequestFromHigherPriorityCoordinator,
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinator},
						{Action: action_CleanUpTransactionsNotYetDispatched},
					},
					Transitions: []Transition{{
						To: State_Closing_Flush,
						If: guard_HasUnconfirmedDispatchedTransactions,
					}, {
						To: State_Closing,
						If: statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions),
					}},
				}},
			},
			Event_EndorsementRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Actions: []ActionRule{{
						Action: action_AddEndorsementRequestSenderToEndorserCandidates,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsEndorsementBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectEndorsementBlockHeight}},
				}, {
					Validator: validator_IsPrivateStateDataPendingForEndorsement,
					Actions:   []ActionRule{{Action: action_RejectEndorsementPrivateStateDataPending}},
				}, {
					// A higher-priority node is sending endorsement requests; step down and handle.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						validator_IsEndorsementRequestFromHigherPriorityCoordinator,
					),
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinatorFromEndorsementRequest},
						{Action: action_CleanUpTransactionsNotYetDispatched},
						{Action: action_ClearTimeoutSchedules},
						{Action: action_HandleEndorsementRequest},
					},
					Transitions: []Transition{{
						To: State_Observing,
						If: statemachine.GuardNot(guard_HasTransactionsInflight),
					}, {
						To: State_Closing_Flush,
						If: statemachine.GuardAnd(guard_HasTransactionsInflight, guard_HasUnconfirmedDispatchedTransactions),
					}, {
						To: State_Closing,
						If: statemachine.GuardAnd(guard_HasTransactionsInflight, statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions)),
					}},
				}, {
					// Lower-priority node — this coordinator is active (or becoming active)
					// TODO: There isn't currently any handling of this rejection as it will require more
					// complex routing of the event between the coordinator and transaction state machines.
					// Handling it as a sign for a coordinator to step down for a higher priority coordinator
					// could speed up time to consistency in a network where there are multiple coordinators.
					// Sending an additional heartbeat at this point achieves a largely similar effect.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						statemachine.ValidatorNot(validator_IsEndorsementRequestFromHigherPriorityCoordinator),
					),
					Actions: []ActionRule{
						{Action: action_RejectEndorsementEndorserIsActiveCoordinator},
						{
							Action: action_SendHeartbeat,
							If:     guard_IsCoordinatorEndorserSelectionMode,
						},
					},
				}},
			},
			Event_TransactionsDelegated: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Validator: validator_IsDelegationBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectDelegationRequestBlockHeight}},
				}, {
					// Accept delegations while in Elect so originators are not bounced while we wait.
					Validator: statemachine.ValidatorNot(validator_IsDelegationBlockHeightToleranceExceeded),
					Actions:   []ActionRule{{Action: action_ProcessDelegatedTransactions}},
				}},
			},
			common.Event_TransactionStateTransition: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					// Newly created transactions will pool immediately (if not blocked by dependencies).
					// While we don't start selecting for assembly until we're active, we need to reflect
					// this pooled state by adding them to the pool.
					Validator: validator_TransactionStateTransitionTo(transaction.State_Pooled),
					Actions:   []ActionRule{{Action: action_PoolTransaction}},
				}, {
					// There is a small chance we have come here from State_Closing and still have transactions in terminal
					// states from a previous time of actively coordinating that we haven't cleaned up from memory yet,
					// so we handle that here.
					Validator: validator_TransactionStateTransitionTo(transaction.State_Final),
					Actions:   []ActionRule{{Action: action_CleanUpTransaction}},
				}},
			},
		},
	},
	State_Prepared: {
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateOriginatorActivity},
						{Action: action_IncrementHeartbeatIntervalCounts},
						{Action: action_PropagateHeartbeatIntervalToTransactions},
						{Action: action_SendHeartbeat},
					},
					Transitions: []Transition{{
						To: State_Active,
						If: guard_InactiveGracePeriodExceeded,
					}},
				}},
			},
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_AddEndorsersFromSnapshot,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					// The current active coordinator is still flushing.
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatFromCurrentActiveCoordinator,
						validator_HeartBeatState(common.CoordinatorState_Closing_Flush),
					),
					Actions: []ActionRule{
						{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
						{Action: action_ProcessConfirmedTransactionsFromSnapshot},
					},
				}, {
					// The current active coordinator has closed - we can take over.
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatFromCurrentActiveCoordinator,
						validator_HeartBeatState(common.CoordinatorState_Closing),
					),
					Actions: []ActionRule{
						{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
						{Action: action_ProcessConfirmedTransactionsFromSnapshot},
						{Action: action_ImportStatesAndLocks},
					},
					Transitions: []Transition{{To: State_Active}},
				}, {
					// We're not going to take over if see an active heartbeat from a higher priority coordinator.
					// Clean up any delegations and choose an appropriate state to move back to.
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatFromHigherPriorityCoordinator,
						validator_IsHeartbeatSenderLive,
					),
					Actions: []ActionRule{
						{Action: action_CleanUpTransactionsNotYetDispatched},
						{Action: action_UpdateActiveCoordinator},
					},
					Transitions: []Transition{{
						// Move to observing if we don't have any transactions in flight.
						To: State_Observing,
						If: statemachine.GuardNot(guard_HasTransactionsInflight),
					}, {
						// Flush if we still have unconfirmed transactions from when we were last active.
						To: State_Closing_Flush,
						If: statemachine.GuardAnd(guard_HasTransactionsInflight, guard_HasUnconfirmedDispatchedTransactions),
					}, {
						// Otherwise move back to closing while we wait for our previously confirmed
						// transactions to be cleared from memory.
						To: State_Closing,
						If: statemachine.GuardAnd(guard_HasTransactionsInflight, statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions)),
					}},
				}},
			},
			Event_HandoverRequest: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_IsHandoverRequestFromHigherPriorityCoordinator,
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinator},
						{Action: action_CleanUpTransactionsNotYetDispatched},
					},
					Transitions: []Transition{{
						To: State_Closing_Flush,
						If: guard_HasUnconfirmedDispatchedTransactions,
					}, {
						To: State_Closing,
						If: statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions),
					}},
				}},
			},
			Event_EndorsementRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Actions: []ActionRule{{
						Action: action_AddEndorsementRequestSenderToEndorserCandidates,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsEndorsementBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectEndorsementBlockHeight}},
				}, {
					Validator: validator_IsPrivateStateDataPendingForEndorsement,
					Actions:   []ActionRule{{Action: action_RejectEndorsementPrivateStateDataPending}},
				}, {
					// A higher-priority node is sending endorsement requests; step down and handle.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						validator_IsEndorsementRequestFromHigherPriorityCoordinator,
					),
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinatorFromEndorsementRequest},
						{Action: action_CleanUpTransactionsNotYetDispatched},
						{Action: action_HandleEndorsementRequest},
					},
					Transitions: []Transition{{
						To: State_Observing,
						If: statemachine.GuardNot(guard_HasTransactionsInflight),
					}, {
						To: State_Closing_Flush,
						If: statemachine.GuardAnd(guard_HasTransactionsInflight, guard_HasUnconfirmedDispatchedTransactions),
					}, {
						To: State_Closing,
						If: statemachine.GuardAnd(guard_HasTransactionsInflight, statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions)),
					}},
				}, {
					// Lower-priority — reject so the sender can re-route.
					// TODO: There isn't currently any handling of this rejection as it will require more
					// complex routing of the event between the coordinator and transaction state machines.
					// Handling it as a sign for a coordinator to step down for a higher priority coordinator
					// could speed up time to consistency in a network where there are multiple coordinators.
					// Sending an additional heartbeat at this point achieves a largely similar effect.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						statemachine.ValidatorNot(validator_IsEndorsementRequestFromHigherPriorityCoordinator),
					),
					Actions: []ActionRule{
						{Action: action_RejectEndorsementEndorserIsActiveCoordinator},
						{Action: action_SendHeartbeat, If: guard_IsCoordinatorEndorserSelectionMode},
					},
				}},
			},
			Event_TransactionsDelegated: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Validator: validator_IsDelegationBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectDelegationRequestBlockHeight}},
				}, {
					Validator: statemachine.ValidatorNot(validator_IsDelegationBlockHeightToleranceExceeded),
					Actions:   []ActionRule{{Action: action_ProcessDelegatedTransactions}},
				}},
			},
			common.Event_TransactionStateTransition: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					// Newly created transactions will pool immediately (if not blocked by dependencies).
					// While we don't start selecting for assembly until we're active, we need to reflect
					// this pooled state by adding them to the pool.
					Validator: validator_TransactionStateTransitionTo(transaction.State_Pooled),
					Actions:   []ActionRule{{Action: action_PoolTransaction}},
				}, {
					// There is a small chance we have come here from State_Closing (via State_Elect) and still have transactions in terminal
					// states from a previous time of actively coordinating that we haven't cleaned up from memory yet,
					// so we handle that here.
					Validator: validator_TransactionStateTransitionTo(transaction.State_Final),
					Actions:   []ActionRule{{Action: action_CleanUpTransaction}},
				}},
			},
		},
	},
	State_Active: {
		OnTransitionTo: []ActionRule{
			{Action: action_SetSelfAsActiveCoordinator},
			{Action: action_SendHeartbeat},
			{Action: action_NewSigningIdentity},
			{Action: action_StartDispatchLoop},
			{If: statemachine.GuardNot(guard_HasTransactionAssembling), Action: action_SelectTransaction},
			// Now we're active we can remove any record of inactivity from a previous coordinator
			// We don't increment the counters in this state as we're the active coordinator who is heartbeating.
			{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
		},
		OnTransitionFrom: []ActionRule{
			// No op if already stopped- some transitions need to call this earlier
			{Action: action_StopDispatchLoop},
		},
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateOriginatorActivity},
						{Action: action_PropagateHeartbeatIntervalToTransactions},
						{Action: action_SendHeartbeat},
					},
					Transitions: []Transition{{
						To: State_Idle,
						If: statemachine.GuardNot(guard_HasTransactionsInflight),
					}},
				}},
			},
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_AddEndorsersFromSnapshot,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatFromHigherPriorityCoordinator,
						validator_IsHeartbeatSenderLive,
					),
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinator},
						{Action: action_StopDispatchLoop},
						// Once the dispatch loop is stopped we know there won't be anymore
						// State_Ready_For_Dispatch to State_Dispatched transitions so it is safe to clean up
						{Action: action_CleanUpTransactionsNotYetDispatched},
					},
					// A higher-priority node live node is announcing itself; step down.
					Transitions: []Transition{{
						To: State_Closing_Flush,
						If: guard_HasUnconfirmedDispatchedTransactions,
					}, {
						To: State_Closing,
						If: statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions),
					}},
				}},
			},
			Event_HandoverRequest: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					// A higher-priority node has explicitly requested we step down; treat identically to a preemption heartbeat.
					// The difference is that the other node will watch our flush and take over gracefully
					Validator: validator_IsHandoverRequestFromHigherPriorityCoordinator,
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinator},
						{Action: action_StopDispatchLoop},
						// Once the dispatch loop is stopped we know there won't be anymore
						// State_Ready_For_Dispatch to State_Dispatched transitions so it is safe to clean up
						{Action: action_CleanUpTransactionsNotYetDispatched},
					},
					Transitions: []Transition{{
						To: State_Closing_Flush,
						If: guard_HasUnconfirmedDispatchedTransactions,
					}, {
						To: State_Closing,
						If: statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions),
					}},
				}},
			},
			Event_EndorsementRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Actions: []ActionRule{{
						Action: action_AddEndorsementRequestSenderToEndorserCandidates,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsEndorsementBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectEndorsementBlockHeight}},
				}, {
					Validator: validator_IsPrivateStateDataPendingForEndorsement,
					Actions:   []ActionRule{{Action: action_RejectEndorsementPrivateStateDataPending}},
				}, {
					// A higher-priority node is sending endorsement requests; step down and handle.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						validator_IsEndorsementRequestFromHigherPriorityCoordinator,
					),
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinatorFromEndorsementRequest},
						{Action: action_StopDispatchLoop},
						// Once the dispatch loop is stopped we know there won't be anymore
						// State_Ready_For_Dispatch to State_Dispatched transitions so it is safe to clean up
						{Action: action_CleanUpTransactionsNotYetDispatched},
						{Action: action_HandleEndorsementRequest},
					},
					Transitions: []Transition{{
						To: State_Closing_Flush,
						If: guard_HasUnconfirmedDispatchedTransactions,
					}, {
						To: State_Closing,
						If: statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions),
					}},
				}, {
					// We are both the coordinator and the endorser; handle directly without stepping down.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						validator_IsEndorsementRequestFromSelf,
					),
					Actions: []ActionRule{{Action: action_HandleEndorsementRequest}},
				}, {
					// Lower-priority node — reject so the sender knows this node is the active coordinator.
					// TODO: There isn't currently any handling of this rejection as it will require more
					// complex routing of the event between the coordinator and transaction state machines.
					// Handling it as a sign for a coordinator to step down for a higher priority coordinator
					// could speed up time to consistency in a network where there are multiple coordinators.
					// Sending an additional heartbeat at this point achieves a largely similar effect.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						statemachine.ValidatorNot(validator_IsEndorsementRequestFromHigherPriorityCoordinator),
						statemachine.ValidatorNot(validator_IsEndorsementRequestFromSelf),
					),
					Actions: []ActionRule{
						{Action: action_RejectEndorsementEndorserIsActiveCoordinator},
						{Action: action_SendHeartbeat, If: guard_IsCoordinatorEndorserSelectionMode},
					},
				}},
			},
			Event_TransactionsDelegated: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Validator: validator_IsDelegationBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectDelegationRequestBlockHeight}},
				}, {
					Validator: statemachine.ValidatorNot(validator_IsDelegationBlockHeightToleranceExceeded),
					Actions:   []ActionRule{{Action: action_ProcessDelegatedTransactions}},
				}},
			},
			common.Event_TransactionStateTransition: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// This TX is leaving dispatched after being reverted on chain, cancel any transaction being assembled
					// This could be more nuanced if we could capture the set of potential states that are being removed
					// as part of unwinding the dependency chain, and only repool the transaction once assembly is complete if
					// it is using one of these potential outputs as an input.
					Validator: statemachine.ValidatorAnd(
						validator_TransactionStateTransitionFrom(transaction.State_Dispatched),
						validator_TransactionStateTransitionTo(transaction.State_Pooled, transaction.State_Reverted),
					),
					Actions: []ActionRule{{If: guard_HasTransactionAssembling, Action: action_cancelCurrentlyAssemblingTransaction}},
				}, {
					Validator: validator_TransactionStateTransitionTo(transaction.State_Pooled),
					Actions:   []ActionRule{{Action: action_PoolTransaction}},
				}, {
					Actions: []ActionRule{{If: statemachine.GuardNot(guard_HasTransactionAssembling), Action: action_SelectTransaction}},
				}, {
					Validator: validator_TransactionStateTransitionTo(transaction.State_Ready_For_Dispatch),
					Actions:   []ActionRule{{Action: action_QueueTransactionForDispatch}},
				}, {
					Validator: validator_TransactionStateTransitionFrom(transaction.State_Dispatched),
					Actions:   []ActionRule{{Action: action_NudgeDispatchLoop}},
				}, {
					Validator: validator_TransactionStateTransitionTo(transaction.State_Final, transaction.State_Evicted),
					Actions:   []ActionRule{{Action: action_CleanUpTransaction}},
				}},
			},
			Event_EpochBoundaryReached: {
				// getAndRefreshBlockHeight queues this event when the effective block height advances to a new
				// epoch. We need to rotate the coordinator signing key.
				// If the key has been used AND we have unconfirmed dispatched transactions we need to
				// take the more expensive route of flushing the dispatched transactions before we can
				// start signing with the new key. The dispatch loop must be stopped in order to reliably
				// make this decision. Otherwise we can rotate the key in place and restart the dispatch loop.
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_StopDispatchLoop},
						{If: statemachine.GuardNot(guard_MustFlushToRotateSigningIdentity), Action: action_NewSigningIdentity},
						// Queueing this event gives the coordinator a chance to process any state transition events before
						// the loop restarts, meaning inflightTxns is guaranteed to be up to date.
						{If: statemachine.GuardNot(guard_MustFlushToRotateSigningIdentity), Action: action_QueueRestartDispatchLoop},
					},
					// Transition to Active_Flush for a key-rotation drain: the signing identity has been used
					// but there are still dispatched transactions in-flight that must be confirmed first.
					Transitions: []Transition{{
						To: State_Active_Flush,
						If: guard_MustFlushToRotateSigningIdentity,
					}},
				}},
			},
			Event_RestartDispatchLoop: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{Action: action_StartDispatchLoop}},
				}},
			},
		},
	},
	State_Active_Flush: {
		// Key-rotation flush: this node is still the active coordinator; it is draining dispatched
		// transactions so the signing key can be rotated before the next dispatch.
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateOriginatorActivity},
						{Action: action_PropagateHeartbeatIntervalToTransactions},
						{Action: action_SendHeartbeat},
					},
				}},
			},
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_AddEndorsersFromSnapshot,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					// A higher-priority node live node is announcing itself; step down.
					Validator: statemachine.ValidatorAnd(
						validator_IsHeartbeatFromHigherPriorityCoordinator,
						validator_IsHeartbeatSenderLive,
					),
					Transitions: []Transition{{
						To: State_Closing_Flush,
						Actions: []ActionRule{
							{Action: action_UpdateActiveCoordinator},
							{Action: action_CleanUpTransactionsNotYetDispatched},
						},
					}},
				}},
			},
			Event_HandoverRequest: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					// A higher-priority node has explicitly requested we step down; treat identically to a preemption heartbeat.
					// The difference is that the other node will watch our flush and take over gracefully
					Validator: validator_IsHandoverRequestFromHigherPriorityCoordinator,
					Actions: []ActionRule{
						{Action: action_CleanUpTransactionsNotYetDispatched},
						{Action: action_UpdateActiveCoordinator},
					},
					Transitions: []Transition{{
						To: State_Closing_Flush,
						If: guard_HasUnconfirmedDispatchedTransactions,
					}, {
						To: State_Closing,
						If: statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions),
					}},
				}},
			},
			Event_EndorsementRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Actions: []ActionRule{{
						Action: action_AddEndorsementRequestSenderToEndorserCandidates,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsEndorsementBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectEndorsementBlockHeight}},
				}, {
					Validator: validator_IsPrivateStateDataPendingForEndorsement,
					Actions:   []ActionRule{{Action: action_RejectEndorsementPrivateStateDataPending}},
				}, {
					// A higher-priority node is sending endorsement requests; step down to Closing_Flush and handle.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						validator_IsEndorsementRequestFromHigherPriorityCoordinator,
					),
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinatorFromEndorsementRequest},
						{Action: action_CleanUpTransactionsNotYetDispatched},
						{Action: action_HandleEndorsementRequest},
					},
					Transitions: []Transition{{
						To: State_Closing_Flush,
					}},
				}, {
					// We are both the coordinator and the endorser; handle directly without stepping down.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						validator_IsEndorsementRequestFromSelf,
					),
					Actions: []ActionRule{{Action: action_HandleEndorsementRequest}},
				}, {
					// Lower-priority node — reject so the sender knows this node is the active coordinator.
					// TODO: There isn't currently any handling of this rejection as it will require more
					// complex routing of the event between the coordinator and transaction state machines.
					// Handling it as a sign for a coordinator to step down for a higher priority coordinator
					// could speed up time to consistency in a network where there are multiple coordinators.
					// Sending an additional heartbeat at this point achieves a largely similar effect.
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
						statemachine.ValidatorNot(validator_IsEndorsementRequestFromHigherPriorityCoordinator),
						statemachine.ValidatorNot(validator_IsEndorsementRequestFromSelf),
					),
					Actions: []ActionRule{
						{Action: action_RejectEndorsementEndorserIsActiveCoordinator},
						{Action: action_SendHeartbeat, If: guard_IsCoordinatorEndorserSelectionMode},
					},
				}},
			},
			Event_TransactionsDelegated: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Validator: validator_IsDelegationBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectDelegationRequestBlockHeight}},
				}, {
					// Still the active coordinator — accept delegations normally- we can process transactions, just not dispatch them
					Validator: statemachine.ValidatorNot(validator_IsDelegationBlockHeightToleranceExceeded),
					Actions:   []ActionRule{{Action: action_ProcessDelegatedTransactions}},
				}},
			},
			common.Event_TransactionStateTransition: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// This TX is leaving dispatched after being reverted on chain, cancel any transaction being assembled
					// This could be more nuanced if we could capture the set of potential states that are being removed
					// as part of unwinding the dependency chain, and only repool the transaction once assembly is complete if
					// it is using one of these potential outputs as an input.
					Validator: statemachine.ValidatorAnd(
						validator_TransactionStateTransitionFrom(transaction.State_Dispatched),
						validator_TransactionStateTransitionTo(transaction.State_Pooled, transaction.State_Reverted),
					),
					Actions: []ActionRule{{If: guard_HasTransactionAssembling, Action: action_cancelCurrentlyAssemblingTransaction}},
				}, {
					Validator: validator_TransactionStateTransitionTo(transaction.State_Pooled),
					Actions:   []ActionRule{{Action: action_PoolTransaction}},
				}, {
					Actions: []ActionRule{{If: statemachine.GuardNot(guard_HasTransactionAssembling), Action: action_SelectTransaction}},
				}, {
					Validator: validator_TransactionStateTransitionTo(transaction.State_Ready_For_Dispatch),
					Actions:   []ActionRule{{Action: action_QueueTransactionForDispatch}},
				}, {
					Validator: validator_TransactionStateTransitionTo(transaction.State_Final, transaction.State_Evicted),
					Actions:   []ActionRule{{Action: action_CleanUpTransaction}},
				}, {
					Validator: validator_TransactionStateTransitionFrom(transaction.State_Dispatched),
					Transitions: []Transition{{
						To: State_Active,
						If: statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions),
					}},
				}},
			},
		},
	},
	State_Closing_Flush: {
		// Step-down flush: this node is no longer the active coordinator.
		// It drains dispatched transactions before transitioning to Closing.
		// Send an immediate heartbeat on entry so any node waiting in State_Elect sees the
		// flush acknowledgement without waiting for the next heartbeat interval.
		OnTransitionTo: []ActionRule{
			{Action: action_SendHeartbeatWithLocks},
		},
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateOriginatorActivity},
						{Action: action_IncrementHeartbeatIntervalsSinceStateChange},
						{Action: action_SendHeartbeatWithLocks},
						{Action: action_PropagateHeartbeatIntervalToTransactions},
					},
				}}},
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_AddEndorsersFromSnapshot,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsHeartbeatSenderLive,
					Actions: []ActionRule{
						{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
						{Action: action_UpdateActiveCoordinator},
					},
				}},
			},
			Event_EndorsementRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Actions: []ActionRule{{
						Action: action_AddEndorsementRequestSenderToEndorserCandidates,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsEndorsementBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectEndorsementBlockHeight}},
				}, {
					Validator: validator_IsPrivateStateDataPendingForEndorsement,
					Actions:   []ActionRule{{Action: action_RejectEndorsementPrivateStateDataPending}},
				}, {
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
					),
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinatorFromEndorsementRequest},
						{Action: action_HandleEndorsementRequest},
					},
				}},
			},
			Event_TransactionsDelegated: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Validator: validator_IsDelegationBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectDelegationRequestBlockHeight}},
				}, {
					Validator: statemachine.ValidatorNot(validator_IsDelegationBlockHeightToleranceExceeded),
					Actions: []ActionRule{
						{
							// This node is now higher-priority than the current active coordinator we stepped down for
							If:     guard_IsHigherPriorityThanCurrentActive,
							Action: action_ProcessDelegatedTransactions,
						},
						{
							// This node is lower-priority — reject and include the current active coordinator's identity.
							If:     statemachine.GuardNot(guard_IsHigherPriorityThanCurrentActive),
							Action: action_RejectDelegationRequest,
						},
					},
					Transitions: []Transition{{
						// this is an edge case which would require disagreement within the network at a block range epoch boundary
						To: State_Elect,
						If: guard_IsHigherPriorityThanCurrentActive,
					}},
				}},
			},
			common.Event_TransactionStateTransition: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// We clean up transactions that have either:
					// - moved to final state
					// - reverted on chain with a retryable error- the new coordinator will assemble and dispatch the retry
					Validator: statemachine.ValidatorOr(
						validator_TransactionStateTransitionTo(transaction.State_Final),
						statemachine.ValidatorAnd(
							validator_TransactionStateTransitionFrom(transaction.State_Dispatched),
							validator_TransactionStateTransitionTo(transaction.State_PreAssembly_Blocked, transaction.State_Pooled),
						),
					),
					Actions: []ActionRule{{Action: action_CleanUpTransaction}},
				}, {
					// A transaction has moved out of dispatched state- is the flush complete?
					Validator: validator_TransactionStateTransitionFrom(transaction.State_Dispatched),
					Transitions: []Transition{{
						To: State_Closing,
						If: statemachine.GuardNot(guard_HasUnconfirmedDispatchedTransactions),
					}},
				}},
			},
		},
	},
	State_Closing: {
		// Send an immediate heartbeat on entry so that any node waiting in State_Prepared sees
		// the flush-complete signal without waiting for the next heartbeat interval.
		OnTransitionTo: []ActionRule{{Action: action_SendHeartbeatWithLocks}},
		Events: map[EventType]EventHandlers{
			common.Event_HeartbeatReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					Actions: []ActionRule{{
						Action: action_AddEndorsersFromSnapshot,
						If:     guard_IsCoordinatorEndorserSelectionMode,
					}},
				}, {
					Validator: validator_IsHeartbeatSenderLive,
					Actions: []ActionRule{
						{Action: action_ResetHeartbeatIntervalsSinceLastReceive},
						{Action: action_UpdateActiveCoordinator},
					},
				}}},
			Event_EndorsementRequestReceived: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Actions: []ActionRule{{Action: action_AddEndorsementRequestSenderToEndorserCandidates, If: guard_IsCoordinatorEndorserSelectionMode}},
				}, {
					Validator: validator_IsEndorsementBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectEndorsementBlockHeight}},
				}, {
					Validator: validator_IsPrivateStateDataPendingForEndorsement,
					Actions:   []ActionRule{{Action: action_RejectEndorsementPrivateStateDataPending}},
				}, {
					Validator: statemachine.ValidatorAnd(
						statemachine.ValidatorNot(validator_IsEndorsementBlockHeightToleranceExceeded),
						statemachine.ValidatorNot(validator_IsPrivateStateDataPendingForEndorsement),
					),
					Actions: []ActionRule{
						{Action: action_UpdateActiveCoordinatorFromEndorsementRequest},
						{Action: action_HandleEndorsementRequest},
					},
				}},
			},
			common.Event_HeartbeatInterval: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Actions: []ActionRule{
						{Action: action_UpdateOriginatorActivity},
						{Action: action_IncrementHeartbeatIntervalsSinceStateChange},
						{Action: action_SendHeartbeatWithLocks},
						{Action: action_PropagateHeartbeatIntervalToTransactions},
					},
					Transitions: []Transition{
						{
							To: State_Idle,
							If: statemachine.GuardAnd(
								statemachine.GuardNot(guard_HasTransactionsInflight),
								guard_ClosingGracePeriodExpired,
								guard_InactiveGracePeriodExceeded,
							),
						},
						{
							To: State_Observing,
							If: statemachine.GuardAnd(
								statemachine.GuardNot(guard_HasTransactionsInflight),
								guard_ClosingGracePeriodExpired,
								statemachine.GuardNot(guard_InactiveGracePeriodExceeded),
							),
						},
					},
				}},
			},
			Event_TransactionsDelegated: {
				Match: statemachine.MatchAll,
				Handlers: []EventHandler{{
					// Always runs first: refresh stored block height before any validator reads it.
					Actions: []ActionRule{{Action: action_RefreshBlockHeight}},
				}, {
					Validator: validator_IsDelegationBlockHeightToleranceExceeded,
					Actions:   []ActionRule{{Action: action_RejectDelegationRequestBlockHeight}},
				}, {
					Validator: statemachine.ValidatorNot(validator_IsDelegationBlockHeightToleranceExceeded),
					Actions: []ActionRule{{
						// Current active coordinator is still live and higher priority; redirect the originator.
						If: statemachine.GuardAnd(
							statemachine.GuardNot(guard_IsHigherPriorityThanCurrentActive),
							statemachine.GuardNot(guard_InactiveGracePeriodExceeded),
						),
						Action: action_RejectDelegationRequest,
					}},
					Transitions: []Transition{
						{
							To: State_Elect,
							If: statemachine.GuardAnd(
								guard_IsHigherPriorityThanCurrentActive,
								statemachine.GuardNot(guard_InactiveGracePeriodExceeded),
							),
							Actions: []ActionRule{{Action: action_ProcessDelegatedTransactions}},
						},
						{
							// Move to active if the current active coordinator has gone idle regardless of priority
							To: State_Active,
							If: statemachine.GuardAnd(
								statemachine.GuardNot(guard_IsHigherPriorityThanCurrentActive),
								guard_InactiveGracePeriodExceeded,
							),
						},
					},
				}},
			},
			common.Event_TransactionStateTransition: {
				Match: statemachine.MatchFirst,
				Handlers: []EventHandler{{
					Validator: validator_TransactionStateTransitionTo(transaction.State_Final),
					Actions:   []ActionRule{{Action: action_CleanUpTransaction}},
				}},
			},
		},
	},
}

func (c *coordinator) initializeStateMachineEventLoop(initialState State, eventQueueSize int, priorityEventQueueSize int) {
	c.stateMachineEventLoop = statemachine.NewStateMachineEventLoop(statemachine.StateMachineEventLoopConfig[State, *coordinator]{
		InitialState:           initialState,
		Definitions:            stateDefinitionsMap,
		Entity:                 c,
		EventQueueSize:         eventQueueSize,
		PriorityEventQueueSize: priorityEventQueueSize,
		Name:                   fmt.Sprintf("coordinator-%s", c.contractAddress.String()[0:8]),
		TransitionCallback:     c.onStateTransition,
		PreProcess:             c.preProcessEvent,
	})
}

// preProcessEvent handles events that should be processed before the state machine.
// Returns (true, nil) if the event was fully handled and should not be passed to the state machine.
func (c *coordinator) preProcessEvent(ctx context.Context, entity *coordinator, event common.Event) (bool, error) {
	// Transaction events are propagated to the transaction state machine, not the coordinator state machine
	if transactionEvent, ok := event.(transaction.Event); ok {
		log.L(ctx).Debugf("coordinator propagating event %s to transactions: %s", event.TypeString(), transactionEvent.TypeString())
		return true, c.propagateEventToTransaction(ctx, transactionEvent)
	}
	// heartbeat events from ourself are filtered out
	if heartbeatEvent, ok := event.(*common.HeartbeatReceivedEvent); ok {
		if heartbeatEvent.FromNode == c.nodeName {
			return true, nil
		}
	}
	return false, nil
}

// onStateTransition is called when the state machine transitions to a new state
func (c *coordinator) onStateTransition(ctx context.Context, entity *coordinator, from State, to State, event common.Event) {
	c.heartbeatIntervalsSinceStateChange = 0
}

// QueueEvent asynchronously queues a state machine event for processing.
// Should be called by most Paladin components to ensure memory integrity of
// sequencer state machine and transactions.
func (c *coordinator) QueueEvent(ctx context.Context, event common.Event) {
	c.stateMachineEventLoop.QueueEvent(ctx, event)
}

func (c *coordinator) TryQueueEvent(ctx context.Context, event common.Event) bool {
	return c.stateMachineEventLoop.TryQueueEvent(ctx, event)
}

// Queue a state machine event generated internally for the sequencer loop to process. Is prioritized above
// external event sources
func (c *coordinator) queueEventInternal(ctx context.Context, event common.Event) {
	c.stateMachineEventLoop.QueuePriorityEvent(ctx, event)
}
