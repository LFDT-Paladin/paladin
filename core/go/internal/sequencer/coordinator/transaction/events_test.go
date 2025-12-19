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

package transaction

import (
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestBaseCoordinatorEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &BaseCoordinatorEvent{
		TransactionID: txID,
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestBaseCoordinatorEvent_GetEventTime(t *testing.T) {
	eventTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	event := &BaseCoordinatorEvent{
		BaseEvent: common.BaseEvent{
			EventTime: eventTime,
		},
	}
	assert.Equal(t, eventTime, event.GetEventTime())
}

func TestReceivedEvent_Type(t *testing.T) {
	event := &ReceivedEvent{}
	assert.Equal(t, Event_Received, event.Type())
}

func TestReceivedEvent_TypeString(t *testing.T) {
	event := &ReceivedEvent{}
	assert.Equal(t, "Event_Received", event.TypeString())
}

func TestReceivedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &ReceivedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestReceivedEvent_GetEventTime(t *testing.T) {
	eventTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	event := &ReceivedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: eventTime,
			},
		},
	}
	assert.Equal(t, eventTime, event.GetEventTime())
}

func TestSelectedEvent_Type(t *testing.T) {
	event := &SelectedEvent{}
	assert.Equal(t, Event_Selected, event.Type())
}

func TestSelectedEvent_TypeString(t *testing.T) {
	event := &SelectedEvent{}
	assert.Equal(t, "Event_Selected", event.TypeString())
}

func TestSelectedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &SelectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestAssembleRequestSentEvent_Type(t *testing.T) {
	event := &AssembleRequestSentEvent{}
	assert.Equal(t, Event_AssembleRequestSent, event.Type())
}

func TestAssembleRequestSentEvent_TypeString(t *testing.T) {
	event := &AssembleRequestSentEvent{}
	assert.Equal(t, "Event_AssembleRequestSent", event.TypeString())
}

func TestAssembleRequestSentEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &AssembleRequestSentEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestAssembleSuccessEvent_Type(t *testing.T) {
	event := &AssembleSuccessEvent{}
	assert.Equal(t, Event_Assemble_Success, event.Type())
}

func TestAssembleSuccessEvent_TypeString(t *testing.T) {
	event := &AssembleSuccessEvent{}
	assert.Equal(t, "Event_Assemble_Success", event.TypeString())
}

func TestAssembleSuccessEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestAssembleSuccessEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
	}
	preAssembly := &components.TransactionPreAssembly{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{},
		Verifiers:         []*prototk.ResolvedVerifier{},
	}

	event := &AssembleSuccessEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		PostAssembly: postAssembly,
		PreAssembly:  preAssembly,
		RequestID:    requestID,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, postAssembly, event.PostAssembly)
	assert.Equal(t, preAssembly, event.PreAssembly)
	assert.Equal(t, requestID, event.RequestID)
}

func TestAssembleRevertResponseEvent_Type(t *testing.T) {
	event := &AssembleRevertResponseEvent{}
	assert.Equal(t, Event_Assemble_Revert_Response, event.Type())
}

func TestAssembleRevertResponseEvent_TypeString(t *testing.T) {
	event := &AssembleRevertResponseEvent{}
	assert.Equal(t, "Event_Assemble_Revert_Response", event.TypeString())
}

func TestAssembleRevertResponseEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &AssembleRevertResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestAssembleRevertResponseEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()
	revertReason := "transaction reverted"
	postAssembly := &components.TransactionPostAssembly{
		AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
		RevertReason:   &revertReason,
	}

	event := &AssembleRevertResponseEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		PostAssembly: postAssembly,
		RequestID:    requestID,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, postAssembly, event.PostAssembly)
	assert.Equal(t, requestID, event.RequestID)
}

func TestEndorsedEvent_Type(t *testing.T) {
	event := &EndorsedEvent{}
	assert.Equal(t, Event_Endorsed, event.Type())
}

func TestEndorsedEvent_TypeString(t *testing.T) {
	event := &EndorsedEvent{}
	assert.Equal(t, "Event_Endorsed", event.TypeString())
}

func TestEndorsedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &EndorsedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestEndorsedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()
	endorsement := &prototk.AttestationResult{
		Name:            "test-endorsement",
		AttestationType: prototk.AttestationType_ENDORSE,
		Payload:         pldtypes.RandBytes(32),
	}

	event := &EndorsedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		Endorsement: endorsement,
		RequestID:   requestID,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, endorsement, event.Endorsement)
	assert.Equal(t, requestID, event.RequestID)
}

func TestEndorsedRejectedEvent_Type(t *testing.T) {
	event := &EndorsedRejectedEvent{}
	assert.Equal(t, Event_EndorsedRejected, event.Type())
}

func TestEndorsedRejectedEvent_TypeString(t *testing.T) {
	event := &EndorsedRejectedEvent{}
	assert.Equal(t, "Event_EndorsedRejected", event.TypeString())
}

func TestEndorsedRejectedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &EndorsedRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestEndorsedRejectedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()
	revertReason := "endorsement rejected"
	party := "endorser@testNode"
	attestationRequestName := "test-endorsement"

	event := &EndorsedRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		RevertReason:           revertReason,
		Party:                  party,
		AttestationRequestName: attestationRequestName,
		RequestID:              requestID,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, revertReason, event.RevertReason)
	assert.Equal(t, party, event.Party)
	assert.Equal(t, attestationRequestName, event.AttestationRequestName)
	assert.Equal(t, requestID, event.RequestID)
}

func TestDispatchRequestApprovedEvent_Type(t *testing.T) {
	event := &DispatchRequestApprovedEvent{}
	assert.Equal(t, Event_DispatchRequestApproved, event.Type())
}

func TestDispatchRequestApprovedEvent_TypeString(t *testing.T) {
	event := &DispatchRequestApprovedEvent{}
	assert.Equal(t, "Event_DispatchRequestApproved", event.TypeString())
}

func TestDispatchRequestApprovedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestDispatchRequestApprovedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	requestID := uuid.New()

	event := &DispatchRequestApprovedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		RequestID: requestID,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, requestID, event.RequestID)
}

func TestDispatchRequestRejectedEvent_Type(t *testing.T) {
	event := &DispatchRequestRejectedEvent{}
	assert.Equal(t, Event_DispatchRequestRejected, event.Type())
}

func TestDispatchRequestRejectedEvent_TypeString(t *testing.T) {
	event := &DispatchRequestRejectedEvent{}
	assert.Equal(t, "Event_DispatchRequestRejected", event.TypeString())
}

func TestDispatchRequestRejectedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &DispatchRequestRejectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestCollectedEvent_Type(t *testing.T) {
	event := &CollectedEvent{}
	assert.Equal(t, Event_Collected, event.Type())
}

func TestCollectedEvent_TypeString(t *testing.T) {
	event := &CollectedEvent{}
	assert.Equal(t, "Event_Collected", event.TypeString())
}

func TestCollectedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &CollectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestCollectedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	signerAddress := *pldtypes.RandAddress()

	event := &CollectedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		SignerAddress: signerAddress,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, signerAddress, event.SignerAddress)
}

func TestDispatchedEvent_Type(t *testing.T) {
	event := &DispatchedEvent{}
	assert.Equal(t, Event_Dispatched, event.Type())
}

func TestDispatchedEvent_TypeString(t *testing.T) {
	event := &DispatchedEvent{}
	assert.Equal(t, "Event_Dispatched", event.TypeString())
}

func TestDispatchedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &DispatchedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestNonceAllocatedEvent_Type(t *testing.T) {
	event := &NonceAllocatedEvent{}
	assert.Equal(t, Event_NonceAllocated, event.Type())
}

func TestNonceAllocatedEvent_TypeString(t *testing.T) {
	event := &NonceAllocatedEvent{}
	assert.Equal(t, "Event_NonceAllocated", event.TypeString())
}

func TestNonceAllocatedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &NonceAllocatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestNonceAllocatedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	nonce := uint64(42)

	event := &NonceAllocatedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		Nonce: nonce,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, nonce, event.Nonce)
}

func TestSubmittedEvent_Type(t *testing.T) {
	event := &SubmittedEvent{}
	assert.Equal(t, Event_Submitted, event.Type())
}

func TestSubmittedEvent_TypeString(t *testing.T) {
	event := &SubmittedEvent{}
	assert.Equal(t, "Event_Submitted", event.TypeString())
}

func TestSubmittedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &SubmittedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestSubmittedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	submissionHash := pldtypes.RandBytes32()

	event := &SubmittedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		SubmissionHash: submissionHash,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, submissionHash, event.SubmissionHash)
}

func TestConfirmedEvent_Type(t *testing.T) {
	event := &ConfirmedEvent{}
	assert.Equal(t, Event_Confirmed, event.Type())
}

func TestConfirmedEvent_TypeString(t *testing.T) {
	event := &ConfirmedEvent{}
	assert.Equal(t, "Event_Confirmed", event.TypeString())
}

func TestConfirmedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestConfirmedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	nonce := uint64(42)
	hash := pldtypes.RandBytes32()
	revertReason := pldtypes.HexBytes{0x01, 0x02, 0x03}

	event := &ConfirmedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		Nonce:        nonce,
		Hash:         hash,
		RevertReason: revertReason,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, nonce, event.Nonce)
	assert.Equal(t, hash, event.Hash)
	assert.Equal(t, revertReason, event.RevertReason)
}

func TestDependencyAssembledEvent_Type(t *testing.T) {
	event := &DependencyAssembledEvent{}
	assert.Equal(t, Event_DependencyAssembled, event.Type())
}

func TestDependencyAssembledEvent_TypeString(t *testing.T) {
	event := &DependencyAssembledEvent{}
	assert.Equal(t, "Event_DependencyAssembled", event.TypeString())
}

func TestDependencyAssembledEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &DependencyAssembledEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestDependencyAssembledEvent_Fields(t *testing.T) {
	txID := uuid.New()
	dependencyID := uuid.New()

	event := &DependencyAssembledEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		DependencyID: dependencyID,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, dependencyID, event.DependencyID)
}

func TestDependencyRevertedEvent_Type(t *testing.T) {
	event := &DependencyRevertedEvent{}
	assert.Equal(t, Event_DependencyReverted, event.Type())
}

func TestDependencyRevertedEvent_TypeString(t *testing.T) {
	event := &DependencyRevertedEvent{}
	assert.Equal(t, "Event_DependencyReverted", event.TypeString())
}

func TestDependencyRevertedEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &DependencyRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestDependencyRevertedEvent_Fields(t *testing.T) {
	txID := uuid.New()
	dependencyID := uuid.New()

	event := &DependencyRevertedEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		DependencyID: dependencyID,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, dependencyID, event.DependencyID)
}

func TestDependencyReadyEvent_Type(t *testing.T) {
	event := &DependencyReadyEvent{}
	assert.Equal(t, Event_DependencyReady, event.Type())
}

func TestDependencyReadyEvent_TypeString(t *testing.T) {
	event := &DependencyReadyEvent{}
	assert.Equal(t, "Event_DependencyReady", event.TypeString())
}

func TestDependencyReadyEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &DependencyReadyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestDependencyReadyEvent_Fields(t *testing.T) {
	txID := uuid.New()
	dependencyID := uuid.New()

	event := &DependencyReadyEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		DependencyID: dependencyID,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, dependencyID, event.DependencyID)
}

func TestRequestTimeoutIntervalEvent_Type(t *testing.T) {
	event := &RequestTimeoutIntervalEvent{}
	assert.Equal(t, Event_RequestTimeoutInterval, event.Type())
}

func TestRequestTimeoutIntervalEvent_TypeString(t *testing.T) {
	event := &RequestTimeoutIntervalEvent{}
	assert.Equal(t, "Event_RequestTimeoutInterval", event.TypeString())
}

func TestRequestTimeoutIntervalEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &RequestTimeoutIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestStateTransitionEvent_Type(t *testing.T) {
	event := &StateTransitionEvent{}
	assert.Equal(t, Event_StateTransition, event.Type())
}

func TestStateTransitionEvent_TypeString(t *testing.T) {
	event := &StateTransitionEvent{}
	assert.Equal(t, "Event_StateTransition", event.TypeString())
}

func TestStateTransitionEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &StateTransitionEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestStateTransitionEvent_Fields(t *testing.T) {
	txID := uuid.New()
	fromState := State_Pooled
	toState := State_Assembling

	event := &StateTransitionEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		FromState: fromState,
		ToState:   toState,
	}

	assert.Equal(t, txID, event.GetTransactionID())
	assert.Equal(t, fromState, event.FromState)
	assert.Equal(t, toState, event.ToState)
}

func TestHeartbeatIntervalEvent_Type(t *testing.T) {
	event := &HeartbeatIntervalEvent{}
	assert.Equal(t, Event_HeartbeatInterval, event.Type())
}

func TestHeartbeatIntervalEvent_TypeString(t *testing.T) {
	event := &HeartbeatIntervalEvent{}
	assert.Equal(t, "Event_HeartbeatInterval", event.TypeString())
}

func TestHeartbeatIntervalEvent_GetTransactionID(t *testing.T) {
	txID := uuid.New()
	event := &HeartbeatIntervalEvent{
		BaseCoordinatorEvent: BaseCoordinatorEvent{
			TransactionID: txID,
		},
	}
	assert.Equal(t, txID, event.GetTransactionID())
}

func TestEvent_InterfaceCompliance(t *testing.T) {
	// Test that all events with BaseCoordinatorEvent implement the Event interface
	txID := uuid.New()
	events := []Event{
		&ReceivedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&SelectedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&AssembleRequestSentEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&AssembleSuccessEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&AssembleRevertResponseEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&EndorsedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&EndorsedRejectedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&DispatchRequestApprovedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&DispatchRequestRejectedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&CollectedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&DispatchedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&NonceAllocatedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&SubmittedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&ConfirmedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&DependencyAssembledEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&DependencyRevertedEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&DependencyReadyEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&RequestTimeoutIntervalEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&StateTransitionEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
		&HeartbeatIntervalEvent{
			BaseCoordinatorEvent: BaseCoordinatorEvent{
				TransactionID: txID,
			},
		},
	}

	for _, event := range events {
		eventType := event.Type()
		assert.NotNil(t, eventType)

		typeString := event.TypeString()
		assert.NotEmpty(t, typeString)

		assert.Equal(t, txID, event.GetTransactionID())

		_ = event.GetEventTime()
	}
}
