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
	"github.com/hyperledger-firefly/signer/pkg/abi"
)

func (n *Noto) BuildReceipt(ctx context.Context, req *prototk.BuildReceiptRequest) (res *prototk.BuildReceiptResponse, err error) {
	receipt := &types.NotoDomainReceipt{}

	infoStates := n.filterSchema(req.InfoStates, []string{n.dataSchemaV0.Id, n.dataSchemaV1.Id, n.dataSchemaV2.Id})
	var variant pldtypes.HexUint64
	if len(infoStates) > 0 {
		// We have at least one data state (the manifest has already been removed).
		// A transaction that prepares a lock spend and cancel has two:
		// 1. the unlock data
		// 2. the transaction's own receipt data
		// So we take the last one, and the variant from it.
		info, err := n.unmarshalInfo(infoStates[len(infoStates)-1].StateDataJson)
		if err != nil {
			return nil, err
		}
		variant = info.Variant

		// Whether that last data state is the receipt data depends on what we have:
		// 1. V0 - there is no manifest, and every party receives every data state, so it is the receipt data.
		// 2. V1+, and we have every state the manifest lists for us. So we know the last data state is the receipt data.
		// 3. V1+, and the manifest shows we're missing some states (or the whole manifest was missing). Don't use it
		// Note: Case (3) is only possible if we're asked for a receipt before waiting for all states to arrive.
		haveRequiredStates := true
		if req.UnavailableStates && variant != types.NotoVariantV0 {
			if haveRequiredStates, err = n.hasRequiredStates(ctx, req); err != nil {
				return nil, err // failed to decode the manifest, or to look up our keys
			}
		}
		if haveRequiredStates {
			receipt.Data = info.Data
		} else {
			log.L(ctx).Debugf("Omitting data from receipt for transaction %s as states it requires are unavailable", req.TransactionId)
		}

		// The sender is the same in every data state, as they are all built from this transaction
		if info.From != nil {
			receipt.Sender = info.From
		}
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
	if variant == types.NotoVariantV0 {
		receipt.LockInfo, err = n.receiptLockInfoV0(ctx, req, &receipt.States, receipt.Data)
	} else if len(receipt.States.UpdatedLockInfo) > 0 {
		receipt.LockInfo, err = n.receiptLockInfoV1V2(ctx, req, variant)
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

// hasRequiredStates returns true if the manifest is available, along with every state it lists for this node
func (n *Noto) hasRequiredStates(ctx context.Context, req *prototk.BuildReceiptRequest) (bool, error) {
	manifestState := n.findManifest(req.InfoStates)
	if manifestState == nil {
		return false, nil
	}
	requiredStateIDs, err := n.requiredStateIDs(ctx, manifestState)
	if err != nil {
		return false, err
	}
	available := make(map[string]bool)
	for _, states := range [][]*prototk.EndorsableState{req.InputStates, req.ReadStates, req.OutputStates, req.InfoStates} {
		for _, s := range states {
			available[s.Id] = true
		}
	}
	for _, requiredStateID := range requiredStateIDs {
		if !available[requiredStateID] {
			return false, nil
		}
	}
	return true, nil
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

func (n *Noto) receiptLockInfoV0(ctx context.Context, req *prototk.BuildReceiptRequest, receiptStates *types.ReceiptStates, receiptData pldtypes.HexBytes) (lockInfo *types.ReceiptLockInfo, err error) {
	var unlockInterfaceABI abi.ABI
	var paramsJSON []byte
	lockInfoStates := n.filterSchema(req.InfoStates, []string{n.lockInfoSchemaV0.Id})
	if len(lockInfoStates) == 1 {
		var lock *types.NotoLockInfo_V0
		lock, err = n.unmarshalLockV0(lockInfoStates[0].StateDataJson)
		if err == nil {
			lockInfo = &types.ReceiptLockInfo{LockID: lock.LockID}
			if !lock.Delegate.IsZero() {
				lockInfo.Delegate = lock.Delegate
			}
		}
	}

	if lockInfo != nil && len(receiptStates.ReadLockedInputs) > 0 && len(receiptStates.PreparedOutputs) > 0 {
		// Old info-based decoding scheme
		var lockID *pldtypes.Bytes32
		var delegate *pldtypes.EthAddress
		lockID, delegate, err = n.extractLockInfoV0(ctx, req.InfoStates, false)
		if err != nil {
			return nil, err
		}
		if lockID != nil {
			lockInfo = &types.ReceiptLockInfo{
				LockID:   *lockID,
				Delegate: delegate, // delegate came directly from the info state in for V0
			}

			unlockInterfaceABI = n.getInterfaceABI(types.NotoVariantV0)
			lockInfo.UnlockFunction = "unlock"
			lockInfo.UnlockParams = map[string]any{
				"txId":          pldtypes.Bytes32UUIDFirst16(uuid.New()).String(), // In V0 we generated a new UUID each time you request a receipt
				"lockedInputs":  n.endorsableStateIDs(ctx, nil, n.filterSchema(req.ReadStates, []string{n.lockedCoinSchema.Id}), false),
				"lockedOutputs": n.endorsableStateIDs(ctx, nil, n.filterSchema(req.InfoStates, []string{n.lockedCoinSchema.Id}), false),
				"outputs":       n.endorsableStateIDs(ctx, nil, n.filterSchema(req.InfoStates, []string{n.coinSchema.Id}), false),
				"signature":     pldtypes.HexBytes{},
				"data":          receiptData, // for V0 we chose to pass the original "data" sent to "prepareUnlock", and decoded here from the info states
			}
			paramsJSON, err = json.Marshal(lockInfo.UnlockParams)
		}
	}
	if err == nil && unlockInterfaceABI != nil {
		unlockFunctionABI := unlockInterfaceABI.Functions()[lockInfo.UnlockFunction]
		lockInfo.UnlockCall, err = unlockFunctionABI.EncodeCallDataJSONCtx(ctx, paramsJSON)
	}
	return lockInfo, err

}

func (n *Noto) receiptLockInfoV1V2(ctx context.Context, req *prototk.BuildReceiptRequest, variant pldtypes.HexUint64) (lockInfo *types.ReceiptLockInfo, err error) {
	unlockInterfaceABI := n.getInterfaceABI(variant)

	// Decode the lock transition
	lt, err := n.validateV1LockTransition(ctx, LOCK_DECODE_ANY, nil, nil, req.InputStates, req.OutputStates)
	if err == nil && lt.newLockState != nil {
		lockInfo = &types.ReceiptLockInfo{
			LockID: lt.newLockInfo.LockID,
		}
	}

	// Prepared locks have a spendTxId, and we add in extra info
	if err == nil && !lt.newLockInfo.SpendTxId.IsZero() {
		lockInfo.SpendTxId = &lt.newLockInfo.SpendTxId
		if lt.newLockInfo.Spender != lt.newLockInfo.Owner {
			lockInfo.Delegate = lt.newLockInfo.Spender
		}

		var lockedInputIDs []string
		if lt.prevLockState == nil {
			// create lock: locked coins are in OutputStates (they were just created)
			lockedInputIDs = n.endorsableStateIDs(ctx, nil, n.filterSchema(req.OutputStates, []string{n.lockedCoinSchema.Id}), false)
		} else {
			// prepare unlock: locked coins are in ReadStates (they were created by a prior lock transaction)
			lockedInputIDs = n.endorsableStateIDs(ctx, nil, n.filterSchema(req.ReadStates, []string{n.lockedCoinSchema.Id}), false)
		}

		// Encode the operation to spend the lock
		var spendLockArgs []byte
		var unlockParamsJSON []byte
		spendLockArgs, err = n.encodeNotoSpendLockArgs(ctx, &types.NotoSpendLockArgs{
			TxId:    lt.newLockInfo.SpendTxId.String(),
			Inputs:  lockedInputIDs,
			Outputs: stringIDs(lt.newLockInfo.SpendOutputs),
			Data:    lt.newLockInfo.SpendData,
			Proof:   pldtypes.HexBytes{}, // have to look back to the createLock/updateLock for the proof
		})
		if err == nil {
			lockInfo.UnlockFunction = "spendLock"
			lockInfo.UnlockParams = map[string]any{
				"lockId":    lockInfo.LockID,
				"spendArgs": pldtypes.HexBytes(spendLockArgs),
				"data":      lt.newLockInfo.SpendData,
			}
		}
		if err == nil {
			unlockParamsJSON, err = json.Marshal(lockInfo.UnlockParams)
		}
		if err == nil {
			unlockFunctionABI := unlockInterfaceABI.Functions()[lockInfo.UnlockFunction]
			lockInfo.UnlockCall, err = unlockFunctionABI.EncodeCallDataJSONCtx(ctx, unlockParamsJSON)
		}

		// Encode the operation to cancel the lock
		var cancelLockArgs []byte
		var cancelParamsJSON []byte
		if err == nil {
			cancelLockArgs, err = n.encodeNotoSpendLockArgs(ctx, &types.NotoSpendLockArgs{
				TxId:    lt.newLockInfo.SpendTxId.String(),
				Inputs:  lockedInputIDs,
				Outputs: stringIDs(lt.newLockInfo.CancelOutputs),
				Data:    lt.newLockInfo.CancelData,
				Proof:   pldtypes.HexBytes{}, // have to look back to the createLock/updateLock for the proof
			})
		}
		if err == nil {
			lockInfo.CancelFunction = "cancelLock"
			lockInfo.CancelParams = map[string]any{
				"lockId":     lockInfo.LockID,
				"cancelArgs": pldtypes.HexBytes(cancelLockArgs),
				"data":       lt.newLockInfo.CancelData,
			}
		}
		if err == nil {
			cancelParamsJSON, err = json.Marshal(lockInfo.CancelParams)
		}
		if err == nil {
			cancelFunctionABI := unlockInterfaceABI.Functions()[lockInfo.CancelFunction]
			lockInfo.CancelCall, err = cancelFunctionABI.EncodeCallDataJSONCtx(ctx, cancelParamsJSON)
		}

	}
	if err != nil {
		return nil, err
	}
	return lockInfo, nil
}
