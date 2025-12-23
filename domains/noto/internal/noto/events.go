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

	var variant pldtypes.HexUint64
	var domainConfig types.NotoParsedConfig
	if err := json.Unmarshal([]byte(req.ContractInfo.GetContractConfigJson()), &domainConfig); err == nil {
		if domainConfig.Variant != 0 {
			variant = domainConfig.Variant
		}
	}

	for _, ev := range req.Events {
		if variant == types.NotoVariantDefault {
			if err := n.handleV1Event(ctx, ev, &res, req); err != nil {
				log.L(ctx).Warnf("Error handling V1 event: %s", err)
				return nil, err
			}
		} else {
			if err := n.handleV0Event(ctx, ev, &res, req); err != nil {
				log.L(ctx).Warnf("Error handling V0 event: %s", err)
				return nil, err
			}
		}
	}
	return &res, nil
}

func (n *Noto) handleV1Event(ctx context.Context, ev *prototk.OnChainEvent, res *prototk.HandleEventBatchResponse, req *prototk.HandleEventBatchRequest) error {
	switch ev.SoliditySignature {
	case eventSignatures[EventTransfer]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var transfer NotoTransfer_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
			txData, err := n.decodeTransactionDataV1(ctx, transfer.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, transfer.TxId, txData.InfoStates, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(transfer.TxId, transfer.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(transfer.TxId, transfer.Outputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed Transfer event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[EventNotoLockCreated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockCreated NotoLockCreated_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockCreated); err == nil {
			txData, err := n.decodeTransactionDataV1(ctx, lockCreated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lockCreated.TxId, txData.InfoStates, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(lockCreated.TxId, lockCreated.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lockCreated.TxId, lockCreated.Outputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lockCreated.TxId, lockCreated.LockedOutputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed LockCreated event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[EventNotoLockUpdated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockUpdated NotoLockUpdated_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockUpdated); err == nil {
			txData, err := n.decodeTransactionDataV1(ctx, lockUpdated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lockUpdated.TxId, txData.InfoStates, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(lockUpdated.TxId, lockUpdated.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lockUpdated.TxId, lockUpdated.Outputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed LockUpdated event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[EventNotoLockSpent], eventSignatures[EventNotoLockCancelled]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockSpent NotoLockSpentOrCancelled_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockSpent); err == nil {
			txData, err := n.decodeTransactionDataV1(ctx, lockSpent.TxData)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lockSpent.TxId, txData.InfoStates, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(lockSpent.TxId, lockSpent.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lockSpent.TxId, lockSpent.Outputs)...)

			if req.ContractInfo != nil {
				var domainConfig *types.NotoParsedConfig
				err = json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
				if err != nil {
					return err
				}
				if domainConfig.IsNotary &&
					domainConfig.NotaryMode == types.NotaryModeHooks.Enum() &&
					!domainConfig.Options.Hooks.PublicAddress.Equals(lockSpent.Spender) {
					err = n.handleNotaryPrivateUnlockV1(ctx, req.StateQueryContext, domainConfig, &lockSpent)
					if err != nil {
						log.L(ctx).Errorf("Failed to handle %s event in batch %s: %s", ev.SoliditySignature, req.BatchId, err)
						return err
					}
				}
			}
		} else {
			log.L(ctx).Warnf("Ignoring malformed %s event in batch %s: %s", ev.SoliditySignature, req.BatchId, err)
		}

	case eventSignatures[EventLockDelegated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockDelegated NotoLockDelegated_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockDelegated); err == nil {
			txData, err := n.decodeTransactionDataV1(ctx, lockDelegated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lockDelegated.TxId, txData.InfoStates, res)
		} else {
			log.L(ctx).Warnf("Ignoring malformed LockDelegated event in batch %s: %s", req.BatchId, err)
		}
	}
	return nil
}

// When notary logic is implemented via Pente, unlock events from the base ledger must be propagated
// back to the Pente hooks
// TODO: this method should not be invoked directly on the event loop, but rather via a queue
func (n *Noto) handleNotaryPrivateUnlock(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, lockedInputs []pldtypes.Bytes32, outputs []pldtypes.Bytes32, spender *pldtypes.EthAddress, data pldtypes.HexBytes, lockID pldtypes.Bytes32) error {
	lockedInputsStr := make([]string, len(lockedInputs))
	for i, input := range lockedInputs {
		lockedInputsStr[i] = input.String()
	}
	unlockedOutputsStr := make([]string, len(outputs))
	for i, output := range outputs {
		unlockedOutputsStr[i] = output.String()
	}

	inputStates, err := n.getStates(ctx, stateQueryContext, n.lockedCoinSchema.Id, lockedInputsStr)
	if err != nil {
		return err
	}
	if len(inputStates) != len(lockedInputsStr) {
		return i18n.NewError(ctx, msgs.MsgMissingStateData, lockedInputs)
	}

	outputStates, err := n.getStates(ctx, stateQueryContext, n.coinSchema.Id, unlockedOutputsStr)
	if err != nil {
		return err
	}
	if len(outputStates) != len(outputs) {
		return i18n.NewError(ctx, msgs.MsgMissingStateData, outputs)
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
			Sender:     spender,
			LockID:     lockID,
			Recipients: recipients,
			Data:       data,
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

func (n *Noto) handleNotaryPrivateUnlockV1(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, unlockEvent *NotoLockSpentOrCancelled_Event) error {
	// V1: lockId is in the event
	return n.handleNotaryPrivateUnlock(ctx, stateQueryContext, domainConfig, unlockEvent.Inputs, unlockEvent.Outputs, unlockEvent.Spender, unlockEvent.TxData, unlockEvent.LockID)
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
