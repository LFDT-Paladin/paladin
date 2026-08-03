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

package types

import "github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"

type ZetoDomainReceipt struct {
	States    ReceiptStates      `json:"states"`
	Transfers []*ReceiptTransfer `json:"transfers,omitempty"`
	// The transaction data supplied to the invocation. Zeto records one info state per transfer
	// entry, distributed only to the sender and that entry's recipient, so a party that received
	// one of several transfers in a transaction sees only their own data here.
	Data pldtypes.HexBytes `json:"data,omitempty"`
}

// ReceiptStates lists the states the transaction consumed and produced. Zeto holds locked and
// unlocked coins in the same schema, distinguished by their "locked" flag, but they are reported
// separately here to match the shape of the Noto domain receipt.
type ReceiptStates struct {
	Inputs        []*ReceiptState `json:"inputs,omitempty"`
	LockedInputs  []*ReceiptState `json:"lockedInputs,omitempty"`
	Outputs       []*ReceiptState `json:"outputs,omitempty"`
	LockedOutputs []*ReceiptState `json:"lockedOutputs,omitempty"`
}

type ReceiptState struct {
	ID     pldtypes.HexBytes `json:"id"`
	Schema pldtypes.Bytes32  `json:"schema"`
	Data   pldtypes.RawJSON  `json:"data"`
}

// ReceiptTransfer describes value moving between owners. Owners are Baby Jubjub public keys rather
// than Ethereum addresses. An absent "from" is a mint, and an absent "to" is a burn (which includes
// withdrawing back to the ERC-20 balance).
type ReceiptTransfer struct {
	From    pldtypes.HexBytes    `json:"from,omitempty"`
	To      pldtypes.HexBytes    `json:"to,omitempty"`
	Amount  *pldtypes.HexUint256 `json:"amount,omitempty"`  // fungible tokens only
	TokenID *pldtypes.HexUint256 `json:"tokenId,omitempty"` // non-fungible tokens only
}
