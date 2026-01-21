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

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"

	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

func guard_HasRevertReason(_ context.Context, reader *Transaction, _ *Transaction) bool {
	return reader.revertReason.String() != ""
}

// stateupdate_Confirmed sets the revert reason from a ConfirmedEvent
// and records the revert time if there's a revert reason
func stateupdate_Confirmed(_ context.Context, state *Transaction, _ *Transaction, _ *Transaction, event common.Event) error {
	confirmedEvent := event.(*ConfirmedEvent)
	state.revertReason = confirmedEvent.RevertReason
	// Record revert time if this is a revert (non-empty revert reason)
	// Note: HexBytes{}.String() returns "0x" not "", so we check length
	if len(confirmedEvent.RevertReason) > 0 {
		now := pldtypes.TimestampNow()
		state.revertTime = &now
	}
	return nil
}
