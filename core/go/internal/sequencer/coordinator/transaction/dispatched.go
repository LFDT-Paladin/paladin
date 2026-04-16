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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

func action_NotifyDispatched(ctx context.Context, t *coordinatorTransaction, _ common.Event) error {
	var txSpec *prototk.TransactionSpecification
	if t.pt.PreAssembly != nil {
		txSpec = t.pt.PreAssembly.TransactionSpecification
	}
	return t.transportWriter.SendDispatched(ctx, t.originator, uuid.New(), txSpec)
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
	return t.transportWriter.SendNonceAssigned(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce)
}

func action_NotifySubmitted(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*SubmittedEvent)
	log.L(ctx).Infof("coordinator transaction applying SubmittedEvent for transaction %s submitted with hash %s", t.pt.ID.String(), e.SubmissionHash.HexString())
	t.latestSubmissionHash = &e.SubmissionHash
	return t.transportWriter.SendTransactionSubmitted(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, &e.SubmissionHash)
}
