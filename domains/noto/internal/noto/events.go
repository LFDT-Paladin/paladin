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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

func (n *Noto) HandleEventBatch(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
	var res prototk.HandleEventBatchResponse
	for _, ev := range req.Events {
		switch ev.SoliditySignature {
		case eventSignatures[NotoTransfer]:
			log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
			var transfer NotoTransfer_Event
			if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
				txData, err := n.decodeTransactionData(ctx, transfer.Data)
				if err != nil {
					return nil, err
				}
				n.recordTransactionInfo(ev, txData.TransactionID, txData.InfoStates, &res)
				res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(txData.TransactionID, transfer.Inputs)...)
				res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txData.TransactionID, transfer.Outputs)...)
			} else {
				log.L(ctx).Warnf("Ignoring malformed NotoTransfer event in batch %s: %s", req.BatchId, err)
			}

		case eventSignatures[NotoLock]:
			log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
			var lock NotoLock_Event
			if err := json.Unmarshal([]byte(ev.DataJson), &lock); err == nil {
				txData, err := n.decodeTransactionData(ctx, lock.Data)
				if err != nil {
					return nil, err
				}
				n.recordTransactionInfo(ev, txData.TransactionID, txData.InfoStates, &res)
				res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(txData.TransactionID, lock.Inputs)...)
				res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txData.TransactionID, lock.Outputs)...)
				res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txData.TransactionID, lock.LockedOutputs)...)
			} else {
				log.L(ctx).Warnf("Ignoring malformed NotoLock event in batch %s: %s", req.BatchId, err)
			}

		case eventSignatures[NotoUnlock]:
			log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
			var unlock NotoUnlock_Event
			if err := json.Unmarshal([]byte(ev.DataJson), &unlock); err == nil {
				txData, err := n.decodeTransactionData(ctx, unlock.Data)
				if err != nil {
					return nil, err
				}
				// If the event has been emitted from a newer Noto contract, it will include the transaction ID that was selected
				// when the domain receipt was built for the prepareUnlock transaction. (Noting that this ID is generated on the
				// fly each time the domain receipt is built, so will differ if repeat ptx_getDomainReceipt calls are made.)

				// txData.TransactionID will always be set to a fallback value since the transaction ID is not encoded into the
				// data parameter of the calldata for the unlock function. This means that for older contracts it is not possible
				// to correlate between the unlock transaction included in the domain receipt for the prepareUnlock and receipt that gets
				// inserted when the unlock is indexed.

				// Some of the other Noto Events include a transaction ID in newer contracts, but since the transaction ID is always
				// available from the transaction data, it doesn't make sense to start checking the event as well. A future version of
				// Noto will move towards greater consistency and less duplication.
				txID := txData.TransactionID
				if !unlock.TxId.IsZero() {
					txID = unlock.TxId
				}
				n.recordTransactionInfo(ev, txID, txData.InfoStates, &res)
				res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(txID, unlock.LockedInputs)...)
				res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txID, unlock.LockedOutputs)...)
				res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txID, unlock.Outputs)...)

				var domainConfig *types.NotoParsedConfig
				err = json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
				if err != nil {
					return nil, err
				}
				if domainConfig.IsNotary &&
					domainConfig.NotaryMode == types.NotaryModeHooks.Enum() &&
					!domainConfig.Options.Hooks.PublicAddress.Equals(unlock.Sender) {
					err = n.handleNotaryPrivateUnlock(ctx, req.StateQueryContext, domainConfig, &unlock)
					if err != nil {
						// Should all errors cause retry?
						log.L(ctx).Errorf("Failed to handle NotoUnlock event in batch %s: %s", req.BatchId, err)
						return nil, err
					}
				}
			} else {
				log.L(ctx).Warnf("Ignoring malformed NotoUnlock event in batch %s: %s", req.BatchId, err)
			}

		case eventSignatures[NotoUnlockPrepared]:
			log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
			var unlockPrepared NotoUnlockPrepared_Event
			if err := json.Unmarshal([]byte(ev.DataJson), &unlockPrepared); err == nil {
				txData, err := n.decodeTransactionData(ctx, unlockPrepared.Data)
				if err != nil {
					return nil, err
				}
				n.recordTransactionInfo(ev, txData.TransactionID, txData.InfoStates, &res)
				res.ReadStates = append(res.ReadStates, n.parseStatesFromEvent(txData.TransactionID, unlockPrepared.LockedInputs)...)
			} else {
				log.L(ctx).Warnf("Ignoring malformed NotoUnlockPrepared event in batch %s: %s", req.BatchId, err)
			}

		case eventSignatures[NotoLockDelegated]:
			log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
			var lockDelegated NotoLockDelegated_Event
			if err := json.Unmarshal([]byte(ev.DataJson), &lockDelegated); err == nil {
				txData, err := n.decodeTransactionData(ctx, lockDelegated.Data)
				if err != nil {
					return nil, err
				}
				n.recordTransactionInfo(ev, txData.TransactionID, txData.InfoStates, &res)
			} else {
				log.L(ctx).Warnf("Ignoring malformed NotoLockDelegated event in batch %s: %s", req.BatchId, err)
			}
		}
	}
	return &res, nil
}

// When notary logic is implemented via Pente, unlock events from the base ledger must be propagated back to the Pente hooks
// TODO: this method should not be invoked directly on the event loop, but rather via a queue
func (n *Noto) handleNotaryPrivateUnlock(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, unlock *NotoUnlock_Event) error {
	lockedInputs := make([]string, len(unlock.LockedInputs))
	for i, input := range unlock.LockedInputs {
		lockedInputs[i] = input.String()
	}
	unlockedOutputs := make([]string, len(unlock.Outputs))
	for i, output := range unlock.Outputs {
		unlockedOutputs[i] = output.String()
	}

	inputStates, err := n.getStates(ctx, stateQueryContext, n.lockedCoinSchema.Id, lockedInputs)
	if err != nil {
		return err
	}
	if len(inputStates) != len(lockedInputs) {
		return i18n.NewError(ctx, msgs.MsgMissingStateData, unlock.LockedInputs)
	}

	outputStates, err := n.getStates(ctx, stateQueryContext, n.coinSchema.Id, unlockedOutputs)
	if err != nil {
		return err
	}
	if len(outputStates) != len(unlock.Outputs) {
		return i18n.NewError(ctx, msgs.MsgMissingStateData, unlock.Outputs)
	}

	var lockID pldtypes.Bytes32
	for _, state := range inputStates {
		coin, err := n.unmarshalLockedCoin(state.DataJson)
		if err != nil {
			return err
		}
		lockID = coin.LockID
		// TODO: should we check that all inputs have the same lock ID?
		break
	}

	recipients := make([]*ResolvedUnlockRecipient, len(outputStates))
	for i, state := range outputStates {
		coin, err := n.unmarshalCoin(state.DataJson)
		if err != nil {
			return err
		}
		recipients[i] = &ResolvedUnlockRecipient{
			To:     coin.Owner,
			Amount: coin.Amount,
		}
	}

	transactionType, functionABI, paramsJSON, err := n.wrapHookTransaction(
		domainConfig,
		solutils.MustLoadBuild(notoHooksJSON).ABI.Functions()["handleDelegateUnlock"],
		&DelegateUnlockHookParams{
			Sender:     unlock.Sender,
			LockID:     lockID,
			Recipients: recipients,
			Data:       unlock.Data,
		},
	)
	if err != nil {
		return err
	}
	functionABIJSON, err := json.Marshal(functionABI)
	if err != nil {
		return err
	}

	_, err = n.Callbacks.SendTransaction(ctx, &prototk.SendTransactionRequest{
		StateQueryContext: stateQueryContext,
		Transaction: &prototk.TransactionInput{
			Type:            mapSendTransactionType(transactionType),
			From:            domainConfig.NotaryLookup,
			ContractAddress: domainConfig.Options.Hooks.PublicAddress.String(),
			FunctionAbiJson: string(functionABIJSON),
			ParamsJson:      string(paramsJSON),
		},
	})
	return err
}

func (n *Noto) parseStatesFromEvent(txID pldtypes.Bytes32, states []pldtypes.Bytes32) []*prototk.StateUpdate {
	refs := make([]*prototk.StateUpdate, len(states))
	for i, state := range states {
		refs[i] = &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txID.String(),
		}
	}
	return refs
}

func (n *Noto) recordTransactionInfo(ev *prototk.OnChainEvent, txID pldtypes.Bytes32, infoStates []pldtypes.Bytes32, res *prototk.HandleEventBatchResponse) {
	res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
		TransactionId: txID.String(),
		Location:      ev.Location,
	})
	for _, state := range infoStates {
		res.InfoStates = append(res.InfoStates, &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txID.String(),
		})
	}
}
