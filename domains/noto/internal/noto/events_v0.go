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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

func (n *Noto) handleV0Event(ctx context.Context, ev *prototk.OnChainEvent, res *prototk.HandleEventBatchResponse, req *prototk.HandleEventBatchRequest) error {
	switch ev.SoliditySignature {
	case eventSignaturesV0[EventNotoTransfer]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var transfer NotoTransfer_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
			txData, err := n.decodeTransactionDataV0(ctx, transfer.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData.TransactionID, txData.InfoStates, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(txData.TransactionID, transfer.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txData.TransactionID, transfer.Outputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoTransfer event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[EventNotoLock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lock NotoLock_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lock); err == nil {
			txData, err := n.decodeTransactionDataV0(ctx, lock.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData.TransactionID, txData.InfoStates, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(lock.TxId, lock.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.Outputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.LockedOutputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[EventNotoUnlock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlock NotoUnlock_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlock); err == nil {
			txData, err := n.decodeTransactionDataV0(ctx, unlock.Data)
			if err != nil {
				return err
			}
			// Noto V0 included some unversioned changes. If the event has been emitted from a newer Noto V0 contract, it will
			// include the transaction ID that was selected when the domain receipt was built for the prepareUnlock transaction.
			// (Noting that this ID is generated on the fly each time the domain receipt is built, so will differ if
			// repeat ptx_getDomainReceipt calls are made.)

			// txData.TransactionID will always be set to a fallback value since the transaction ID is not encoded into the
			// data parameter of the calldata for the unlock function. This means that for older contracts it is not possible
			// to correlate between the unlock transaction included in the domain receipt for the prepareUnlock and receipt that gets
			// inserted when the unlock is indexed.

			// Some of the other Noto Events include a transaction ID in newer V0 contracts, but since the transaction ID is always
			// available from the transaction data, it doesn't make sense to start checking the event as well.
			// V1 has solved this duplication/inconsistency problem by ensuring that the transaction ID is always available in the event.
			txID := txData.TransactionID
			if !unlock.TxId.IsZero() {
				txID = unlock.TxId
			}
			n.recordTransactionInfo(ev, txID, txData.InfoStates, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(txID, unlock.LockedInputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txID, unlock.LockedOutputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txID, unlock.Outputs)...)

			var domainConfig *types.NotoParsedConfig
			err = json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
			if err != nil {
				return err
			}
			if domainConfig.IsNotary &&
				domainConfig.NotaryMode == types.NotaryModeHooks.Enum() &&
				!domainConfig.Options.Hooks.PublicAddress.Equals(&unlock.Sender) {
				err = n.handleNotaryPrivateUnlockV0(ctx, req.StateQueryContext, domainConfig, &unlock)
				if err != nil {
					log.L(ctx).Errorf("Failed to handle NotoUnlock event in batch %s: %s", req.BatchId, err)
					return err
				}
			}
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[EventNotoUnlockPrepared]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlockPrepared NotoUnlockPrepared_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlockPrepared); err == nil {
			txData, err := n.decodeTransactionDataV0(ctx, unlockPrepared.Data)
			if err != nil {
				return err
			}
			// Transaction ID is not available in the event data, so we must decode it from the data field
			n.recordTransactionInfo(ev, txData.TransactionID, txData.InfoStates, res)
			res.ReadStates = append(res.ReadStates, n.parseStatesFromEvent(txData.TransactionID, unlockPrepared.LockedInputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlockPrepared event in batch %s: %s", req.BatchId, err)
		}

	case eventSignaturesV0[EventNotoLockDelegated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockDelegated NotoLockDelegated_V0_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockDelegated); err == nil {
			txData, err := n.decodeTransactionDataV0(ctx, lockDelegated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, txData.TransactionID, txData.InfoStates, res)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLockDelegated event in batch %s: %s", req.BatchId, err)
		}
	}
	return nil
}

func (n *Noto) handleNotaryPrivateUnlockV0(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, unlock *NotoUnlock_V0_Event) error {
	// V0: extract lockId from locked coin states
	var lockID pldtypes.Bytes32
	lockedInputsStr := make([]string, len(unlock.LockedInputs))
	for i, input := range unlock.LockedInputs {
		lockedInputsStr[i] = input.String()
	}
	inputStates, err := n.getStates(ctx, stateQueryContext, n.lockedCoinSchema.Id, lockedInputsStr)
	if err != nil {
		return err
	}
	if len(inputStates) > 0 {
		coin, err := n.unmarshalLockedCoin(inputStates[0].DataJson)
		if err != nil {
			return err
		}
		lockID = coin.LockID
		// TODO: should we check that all inputs have the same lock ID?
	}
	return n.handleNotaryPrivateUnlock(ctx, stateQueryContext, domainConfig, unlock.LockedInputs, unlock.Outputs, &unlock.Sender, unlock.Data, lockID)
}
