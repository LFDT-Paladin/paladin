/*
 * Copyright © 2024 Kaleido, Inc.
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

package noto

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

func (n *Noto) BuildReceipt(ctx context.Context, req *prototk.BuildReceiptRequest) (res *prototk.BuildReceiptResponse, err error) {
	receipt := &types.NotoDomainReceipt{}

	infoStates := n.filterSchema(req.InfoStates, []string{n.dataSchemaV0.Id, n.dataSchemaV1.Id})
	var variant pldtypes.HexUint64
	if len(infoStates) == 1 {
		info, err := n.unmarshalInfo(infoStates[0].StateDataJson)
		if err != nil {
			return nil, err
		}
		receipt.Data = info.Data
		variant = info.Variant
	}

	receipt.States.Inputs, err = n.receiptStates(ctx, n.filterSchema(req.InputStates, []string{n.coinSchema.Id}))
	if err == nil {
		receipt.States.LockedInputs, err = n.receiptStates(ctx, n.filterSchema(req.InputStates, []string{n.lockedCoinSchema.Id}))
	}
	if err == nil {
		receipt.States.Outputs, err = n.receiptStates(ctx, n.filterSchema(req.OutputStates, []string{n.coinSchema.Id}))
	}
	if err == nil {
		receipt.States.UpdatedLockInfo, err = n.receiptStates(ctx, n.filterSchema(req.OutputStates, []string{n.lockInfoSchemaV1.Id}))
	}
	if err == nil {
		receipt.States.LockedOutputs, err = n.receiptStates(ctx, n.filterSchema(req.OutputStates, []string{n.lockedCoinSchema.Id}))
	}
	if err == nil {
		receipt.States.ReadInputs, err = n.receiptStates(ctx, n.filterSchema(req.ReadStates, []string{n.coinSchema.Id}))
	}
	if err == nil {
		receipt.States.ReadLockedInputs, err = n.receiptStates(ctx, n.filterSchema(req.ReadStates, []string{n.lockedCoinSchema.Id}))
	}
	if err == nil {
		receipt.States.PreparedOutputs, err = n.receiptStates(ctx, n.filterSchema(req.InfoStates, []string{n.coinSchema.Id}))
	}
	if err == nil {
		receipt.States.PreparedLockedOutputs, err = n.receiptStates(ctx, n.filterSchema(req.InfoStates, []string{n.lockedCoinSchema.Id}))
	}
	if err != nil {
		return nil, err
	}

	// For prepareUnlock, createMintLock, and prepareBurnUnlock transactions, include the encoded "unlock"
	// call that can be used to unlock the coins.
	var unlockInterfaceABI abi.ABI
	var paramsJSON []byte
	if variant == types.NotoVariantDefault && len(receipt.States.UpdatedLockInfo) > 0 {
		// New style lock transition
		var lt *lockTransition
		lt, err = n.validateV1LockTransition(ctx, LOCK_DECODE_ANY, nil, nil, req.InputStates, req.OutputStates)
		var notoUnlockOpEncoded []byte
		if err == nil && lt.newLockState != nil {
			receipt.LockInfo = &types.ReceiptLockInfo{
				LockID: lt.newLockInfo.LockID,
			}
		}
		if err == nil && !lt.newLockInfo.SpendTxId.IsZero() {
			receipt.LockInfo.SpendTxId = &lt.newLockInfo.SpendTxId
			// We generated a spendable lock in this transaction
			unlockInterfaceABI = n.getInterfaceABI(types.NotoVariantDefault)
			notoUnlockOpEncoded, err = n.encodeNotoUnlockOperation(ctx, receipt.LockInfo.LockID, &types.NotoUnlockOperation{
				TxId:    lt.newLockInfo.SpendTxId.String(),
				Inputs:  endorsableStateIDs(n.filterSchema(req.ReadStates, []string{n.lockedCoinSchema.Id})),
				Outputs: stringIDs(lt.newLockInfo.SpendOutputs),
				Data:    lt.newLockInfo.SpendData,
				Proof:   pldtypes.HexBytes{}, // have to look back to the createLock/updateLock for the proof
			})
		}
		if err == nil && notoUnlockOpEncoded != nil {
			receipt.LockInfo.UnlockFunction = "spendLock"
			receipt.LockInfo.UnlockParams = map[string]any{
				"lockId":      receipt.LockInfo.LockID,
				"spendInputs": pldtypes.HexBytes(notoUnlockOpEncoded),
				"data":        receipt.Data, // the inner data is what matters here
			}
			paramsJSON, err = json.Marshal(receipt.LockInfo.UnlockParams)
		}
	} else if variant == types.NotoVariantLegacy {

		lockInfoStates := n.filterSchema(req.InfoStates, []string{n.lockInfoSchemaV0.Id})
		if len(lockInfoStates) == 1 {
			var lock *types.NotoLockInfo_V0
			lock, err = n.unmarshalLockV0(lockInfoStates[0].StateDataJson)
			if err == nil {
				receipt.LockInfo = &types.ReceiptLockInfo{LockID: lock.LockID}
				if !lock.Delegate.IsZero() {
					receipt.LockInfo.Delegate = lock.Delegate
				}
			}
		}

		if receipt.LockInfo != nil && len(receipt.States.ReadLockedInputs) > 0 && len(receipt.States.PreparedOutputs) > 0 {
			// Old info-based decoding scheme
			var lockID *pldtypes.Bytes32
			var spendTxID *pldtypes.Bytes32
			var delegate *pldtypes.EthAddress
			lockID, spendTxID, delegate, err = n.extractLockInfoV0(ctx, req.InfoStates, false)
			if err != nil {
				return nil, err
			}
			if lockID != nil {
				receipt.LockInfo = &types.ReceiptLockInfo{
					LockID:    *lockID,
					SpendTxId: spendTxID, // only for V1
					Delegate:  delegate,  // only for V0
				}

				unlockInterfaceABI = n.getInterfaceABI(types.NotoVariantLegacy)
				receipt.LockInfo.UnlockFunction = "unlock"
				receipt.LockInfo.UnlockParams = map[string]any{
					"txId":          pldtypes.Bytes32UUIDFirst16(uuid.New()).String(), // In V0 we generated a new UUID each time you request a receipt
					"lockedInputs":  endorsableStateIDs(n.filterSchema(req.ReadStates, []string{n.lockedCoinSchema.Id})),
					"lockedOutputs": endorsableStateIDs(n.filterSchema(req.InfoStates, []string{n.lockedCoinSchema.Id})),
					"outputs":       endorsableStateIDs(n.filterSchema(req.InfoStates, []string{n.coinSchema.Id})),
					"signature":     pldtypes.HexBytes{},
					"data":          receipt.Data,
				}
				paramsJSON, err = json.Marshal(receipt.LockInfo.UnlockParams)
			}
		}
	}
	if err == nil && unlockInterfaceABI != nil {
		unlockFunctionABI := unlockInterfaceABI.Functions()[receipt.LockInfo.UnlockFunction]
		receipt.LockInfo.UnlockCall, err = unlockFunctionABI.EncodeCallDataJSONCtx(ctx, paramsJSON)
	}
	if err == nil {
		receipt.Transfers, err = n.receiptTransfers(ctx, req)
	}
	var receiptJSON []byte
	if err == nil {
		receiptJSON, err = json.Marshal(receipt)
	}
	if err != nil {
		return nil, err
	}

	return &prototk.BuildReceiptResponse{
		ReceiptJson: string(receiptJSON),
	}, nil
}

func (n *Noto) receiptStates(ctx context.Context, states []*prototk.EndorsableState) ([]*types.ReceiptState, error) {
	coins := make([]*types.ReceiptState, len(states))
	for i, state := range states {
		id, err := pldtypes.ParseHexBytes(ctx, state.Id)
		if err != nil {
			return nil, err
		}
		schemaID, err := pldtypes.ParseBytes32Ctx(ctx, state.SchemaId)
		if err != nil {
			return nil, err
		}
		coins[i] = &types.ReceiptState{
			ID:     id,
			Schema: schemaID,
			Data:   pldtypes.RawJSON(state.StateDataJson),
		}
	}
	return coins, nil
}

func (n *Noto) receiptTransfers(ctx context.Context, req *prototk.BuildReceiptRequest) ([]*types.ReceiptTransfer, error) {
	inputCoins, err := n.parseCoinList(ctx, "inputs", n.filterSchema(req.InputStates, []string{n.coinSchema.Id, n.lockedCoinSchema.Id}))
	if err != nil {
		return nil, err
	}
	outputCoins, err := n.parseCoinList(ctx, "outputs", n.filterSchema(req.OutputStates, []string{n.coinSchema.Id, n.lockedCoinSchema.Id}))
	if err != nil {
		return nil, err
	}

	var from *pldtypes.EthAddress
	fromAmount := big.NewInt(0)
	to := make(map[pldtypes.EthAddress]*big.Int)

	parseInput := func(owner *pldtypes.EthAddress, amount *big.Int) bool {
		if from == nil {
			from = owner
		} else if !owner.Equals(from) {
			return false
		}
		fromAmount.Add(fromAmount, amount)
		return true
	}

	parseOutput := func(owner pldtypes.EthAddress, amount *big.Int) bool {
		if owner.Equals(from) {
			fromAmount.Sub(fromAmount, amount)
		} else if toAmount, ok := to[owner]; ok {
			toAmount.Add(toAmount, amount)
		} else {
			to[owner] = amount
		}
		return true
	}

	parsedOK := true
	for _, coin := range inputCoins.coins {
		parsedOK = parsedOK && parseInput(coin.Owner, coin.Amount.Int())
	}
	for _, coin := range inputCoins.lockedCoins {
		parsedOK = parsedOK && parseInput(coin.Owner, coin.Amount.Int())
	}
	for _, coin := range outputCoins.coins {
		parsedOK = parsedOK && parseOutput(*coin.Owner, coin.Amount.Int())
	}
	for _, coin := range outputCoins.lockedCoins {
		parsedOK = parsedOK && parseOutput(*coin.Owner, coin.Amount.Int())
	}
	if !parsedOK {
		log.L(ctx).Warnf("Failed to parse transfer coins")
		return nil, nil
	}

	if len(to) == 0 && from != nil && fromAmount.BitLen() > 0 {
		// special case for burn (no recipients)
		return []*types.ReceiptTransfer{{
			From:   from,
			Amount: (*pldtypes.HexUint256)(fromAmount),
		}}, nil
	}

	transfers := make([]*types.ReceiptTransfer, 0, len(to))
	for owner, amount := range to {
		if amount.BitLen() > 0 {
			transfers = append(transfers, &types.ReceiptTransfer{
				From:   from,
				To:     &owner,
				Amount: (*pldtypes.HexUint256)(amount),
			})
		}
	}
	return transfers, nil
}
