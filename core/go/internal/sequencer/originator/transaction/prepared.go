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

func action_Dispatched(ctx context.Context, t *Transaction, event common.Event) error {
	e := event.(*DispatchedEvent)
	t.signerAddress = &e.SignerAddress
	return nil
}

func action_PreDispatchRequestReceived(ctx context.Context, t *Transaction, event common.Event) error {
	e := event.(*PreDispatchRequestReceivedEvent)
	t.latestPreDispatchRequestID = e.RequestID
	return nil
}

func action_ResendPreDispatchResponse(ctx context.Context, txn *Transaction, _ common.Event) error {
	return action_SendPreDispatchResponse(ctx, txn, nil)
}
