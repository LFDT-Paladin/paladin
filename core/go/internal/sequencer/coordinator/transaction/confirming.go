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
	"context"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

func guard_HasRevertReason(ctx context.Context, txn *Transaction) bool {
	return txn.revertReason.String() != ""
}

// stateupdate_Confirmed sets the revert reason from a ConfirmedEvent
func stateupdate_Confirmed(_ context.Context, txn *Transaction, event common.Event) error {
	confirmedEvent := event.(*ConfirmedEvent)
	txn.revertReason = confirmedEvent.RevertReason
	return nil
}
