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

// stateupdate_Collected sets the signer address from a CollectedEvent
func stateupdate_Collected(_ context.Context, txn *Transaction, event common.Event) error {
	collectedEvent := event.(*CollectedEvent)
	txn.signerAddress = &collectedEvent.SignerAddress
	return nil
}

// stateupdate_NonceAllocated sets the nonce from a NonceAllocatedEvent
func stateupdate_NonceAllocated(_ context.Context, txn *Transaction, event common.Event) error {
	nonceAllocatedEvent := event.(*NonceAllocatedEvent)
	txn.nonce = &nonceAllocatedEvent.Nonce
	return nil
}

// stateupdate_Submitted sets the latest submission hash from a SubmittedEvent
func stateupdate_Submitted(ctx context.Context, txn *Transaction, event common.Event) error {
	submittedEvent := event.(*SubmittedEvent)
	log.L(ctx).Infof("coordinator transaction applying SubmittedEvent for transaction %s submitted with hash %s", txn.ID.String(), submittedEvent.SubmissionHash.HexString())
	txn.latestSubmissionHash = &submittedEvent.SubmissionHash
	return nil
}
