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
)

func action_Collected(_ context.Context, t *Transaction, event common.Event) error {
	e := event.(*CollectedEvent)
	t.signerAddress = &e.SignerAddress
	return nil
}

func action_NonceAllocated(ctx context.Context, t *Transaction, event common.Event) error {
	e := event.(*NonceAllocatedEvent)
	t.nonce = &e.Nonce
	return t.transportWriter.SendNonceAssigned(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, e.Nonce)
}

func action_Submitted(ctx context.Context, t *Transaction, event common.Event) error {
	e := event.(*SubmittedEvent)
	log.L(ctx).Infof("coordinator transaction applying SubmittedEvent for transaction %s submitted with hash %s", t.pt.ID.String(), e.SubmissionHash.HexString())
	t.latestSubmissionHash = &e.SubmissionHash
	return t.transportWriter.SendTransactionSubmitted(ctx, t.pt.ID, t.originatorNode, &t.pt.Address, &e.SubmissionHash)
}
