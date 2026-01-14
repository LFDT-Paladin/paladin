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

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

// stateupdate_Dispatched sets the signer address from a DispatchedEvent
func stateupdate_Dispatched(_ context.Context, txn *Transaction, event common.Event) error {
	dispatchedEvent := event.(*DispatchedEvent)
	txn.signerAddress = &dispatchedEvent.SignerAddress
	return nil
}

// stateupdate_NonceAssigned sets the signer address and nonce from a NonceAssignedEvent
func stateupdate_NonceAssigned(_ context.Context, txn *Transaction, event common.Event) error {
	nonceAssignedEvent := event.(*NonceAssignedEvent)
	txn.signerAddress = &nonceAssignedEvent.SignerAddress
	txn.nonce = &nonceAssignedEvent.Nonce
	return nil
}

// stateupdate_Submitted sets the signer address, nonce, and submission hash from a SubmittedEvent
func stateupdate_Submitted(_ context.Context, txn *Transaction, event common.Event) error {
	submittedEvent := event.(*SubmittedEvent)
	txn.signerAddress = &submittedEvent.SignerAddress
	txn.nonce = &submittedEvent.Nonce
	txn.latestSubmissionHash = &submittedEvent.LatestSubmissionHash
	return nil
}
