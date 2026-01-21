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
	LockInfo  *ReceiptLockInfo   `json:"lockInfo,omitempty"`
	Data      pldtypes.HexBytes  `json:"data,omitempty"`
}

type ReceiptStates struct {
	Inputs                []*ReceiptState `json:"inputs,omitempty"`
	LockedInputs          []*ReceiptState `json:"lockedInputs,omitempty"`
	Outputs               []*ReceiptState `json:"outputs,omitempty"`
	LockedOutputs         []*ReceiptState `json:"lockedOutputs,omitempty"`
	ReadInputs            []*ReceiptState `json:"readInputs,omitempty"`
	ReadLockedInputs      []*ReceiptState `json:"readLockedInputs,omitempty"`
	PreparedOutputs       []*ReceiptState `json:"preparedOutputs,omitempty"`
	PreparedLockedOutputs []*ReceiptState `json:"preparedLockedOutputs,omitempty"`
}

type ReceiptState struct {
	ID     pldtypes.HexBytes `json:"id"`
	Schema pldtypes.Bytes32  `json:"schema"`
	Data   pldtypes.RawJSON  `json:"data"`
}

type ReceiptTransfer struct {
	From    pldtypes.HexBytes    `json:"from,omitempty"`
	To      pldtypes.HexBytes    `json:"to,omitempty"`
	Amount  *pldtypes.HexUint256 `json:"amount"`
	TokenId *pldtypes.HexUint256 `json:"tokenId"`
}

type ReceiptLockInfo struct {
	LockID       pldtypes.Bytes32     `json:"lockId"`
	Delegate     *pldtypes.EthAddress `json:"delegate,omitempty"`     // only set for delegateLock
	UnlockTxId   *pldtypes.Bytes32    `json:"unlockTxId,omitempty"`   // only set for prepareUnlock
	UnlockParams map[string]any       `json:"unlockParams,omitempty"` // only set for prepareUnlock
	UnlockCall   pldtypes.HexBytes    `json:"unlockCall,omitempty"`   // only set for prepareUnlock
}
