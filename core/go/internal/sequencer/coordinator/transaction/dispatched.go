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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

func action_NotifyDispatched(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	msg := &engineProto.TransactionDispatched{
		Id:              uuid.New().String(),
		ContractAddress: t.pt.Address.HexString(),
		Signer:          t.originator,
		TransactionId:   t.pt.ID.String(),
	}
	return t.transportWriter.SendDispatched(ctx, t.originatorNode, msg)
}

// guard_HasDispatchedPublicTransaction reports whether dispatching this transaction will send a public
// transaction (rather than producing new private/prepared transactions), gating whether it counts
// towards the coordinator's dispatch-ahead limit.
func guard_HasDispatchedPublicTransaction(_ context.Context, t *coordinatorTransaction) bool {
	return t.pt.PreparedPublicTransaction != nil &&
		t.pt.PreAssembly.TransactionSpecification.Intent == prototk.TransactionSpecification_SEND_TRANSACTION
}

// action_MarkDispatchedInFlight records this transaction against the coordinator's dispatch-ahead
// count when it enters State_Dispatched. Tracking it here (and clearing it in
// action_ClearDispatchedInFlight on exit) keeps the count exact across revert-and-redispatch, since
// both run synchronously within the state transition.
func action_MarkDispatchedInFlight(_ context.Context, t *coordinatorTransaction, _ common.Event) error {
	if t.setDispatchedInFlight != nil {
		t.setDispatchedInFlight(t.pt.ID, true)
	}
	return nil
}

// action_ClearDispatchedInFlight clears this transaction from the coordinator's dispatch-ahead count
// when it leaves State_Dispatched. It is idempotent, so it is safe even when the transaction was not
// counted on entry (i.e. it did not dispatch a public transaction).
func action_ClearDispatchedInFlight(_ context.Context, t *coordinatorTransaction, _ common.Event) error {
	if t.setDispatchedInFlight != nil {
		t.setDispatchedInFlight(t.pt.ID, false)
	}
	return nil
}

// action_CleanUpAssemblyPayload releases the heavy post-assembly and prepared-dispatch
// payload data after dispatch. PreAssembly is preserved because it holds the
// TransactionSpecification and RequiredVerifiers needed if the transaction reverts
// and must be re-assembled.
func action_CleanUpAssemblyPayload(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("cleaning up assembly payload for dispatched transaction %s", t.pt.ID.String())
	t.pt.CleanUpPostAssemblyData()
	return nil
}

func action_NotifyCollected(_ context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*CollectedEvent)
	t.signerAddress = &e.SignerAddress
	return nil
}

func action_NotifyNonceAllocated(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*NonceAllocatedEvent)
	t.nonce = &e.Nonce
	return t.transportWriter.SendNonceAssigned(ctx, t.originatorNode, &engineProto.NonceAssigned{
		Id:              uuid.New().String(),
		TransactionId:   t.pt.ID.String(),
		ContractAddress: t.pt.Address.HexString(),
		Nonce:           int64(e.Nonce),
	})
}

func action_NotifySubmitted(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*SubmittedEvent)
	log.L(ctx).Infof("coordinator transaction applying SubmittedEvent for transaction %s submitted with hash %s", t.pt.ID.String(), e.SubmissionHash.HexString())
	t.latestSubmissionHash = &e.SubmissionHash
	return t.transportWriter.SendTransactionSubmitted(ctx, t.originatorNode, &engineProto.TransactionSubmitted{
		Id:              uuid.New().String(),
		TransactionId:   t.pt.ID.String(),
		ContractAddress: t.pt.Address.HexString(),
		Hash:            e.SubmissionHash.Bytes(),
	})
}
