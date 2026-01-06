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
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	notosmt "github.com/LFDT-Paladin/paladin/domains/noto/internal/noto/smt"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/smt"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/core"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/node"
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
		if variant == types.NotoVariantDefault || variant == types.NotoVariantNullifier {
			if err := n.handleV1Event(ctx, ev, &res, req, variant == types.NotoVariantNullifier); err != nil {
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

func (n *Noto) handleV1Event(ctx context.Context, ev *prototk.OnChainEvent, res *prototk.HandleEventBatchResponse, req *prototk.HandleEventBatchRequest, useNullifier bool) error {
	var smtForStates *smt.MerkleTreeSpec
	var err error
	if useNullifier {
		smtName := notosmt.MerkleTreeName(req.ContractInfo.ContractAddress)
		hasher := &smt.Keccak256Hasher{}
		smtForStates, err = smt.NewMerkleTreeSpec(ctx, smtName, smt.StatesTree, notosmt.SMT_HEIGHT_UTXO, hasher, n.Callbacks, n.merkleTreeRootSchema.Id, n.merkleTreeNodeSchema.Id, req.StateQueryContext)
		if err != nil {
			return err
		}
	}

	switch ev.SoliditySignature {
	case eventSignatures[NotoTransfer]:
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
			fmt.Printf("spent states: %+v\n", res.SpentStates)
			fmt.Printf("confirmed states: %+v\n", res.ConfirmedStates)
			if useNullifier {
				n.updateMerkleTree(ctx, smtForStates.Tree, smtForStates.Storage, transfer.TxId, convertToUint256(transfer.Outputs))
			}
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoTransfer event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[NotoLock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lock NotoLock_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lock); err == nil {
			txData, err := n.decodeTransactionDataV1(ctx, lock.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lock.TxId, txData.InfoStates, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(lock.TxId, lock.Inputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.Outputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(lock.TxId, lock.LockedOutputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[NotoUnlock]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlock NotoUnlock_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlock); err == nil {
			txData, err := n.decodeTransactionDataV1(ctx, unlock.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, unlock.TxId, txData.InfoStates, res)
			res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(unlock.TxId, unlock.LockedInputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlock.TxId, unlock.LockedOutputs)...)
			res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(unlock.TxId, unlock.Outputs)...)

			var domainConfig *types.NotoParsedConfig
			err = json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
			if err != nil {
				return err
			}
			if domainConfig.IsNotary &&
				domainConfig.NotaryMode == types.NotaryModeHooks.Enum() &&
				!domainConfig.Options.Hooks.PublicAddress.Equals(unlock.Sender) {
				err = n.handleNotaryPrivateUnlockV1(ctx, req.StateQueryContext, domainConfig, &unlock)
				if err != nil {
					log.L(ctx).Errorf("Failed to handle NotoUnlock event in batch %s: %s", req.BatchId, err)
					return err
				}
			}
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlock event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[NotoUnlockPrepared]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var unlockPrepared NotoUnlockPrepared_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &unlockPrepared); err == nil {
			txData, err := n.decodeTransactionDataV1(ctx, unlockPrepared.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, unlockPrepared.TxId, txData.InfoStates, res)
			res.ReadStates = append(res.ReadStates, n.parseStatesFromEvent(unlockPrepared.TxId, unlockPrepared.LockedInputs)...)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoUnlockPrepared event in batch %s: %s", req.BatchId, err)
		}

	case eventSignatures[NotoLockDelegated]:
		log.L(ctx).Infof("Processing '%s' event in batch %s", ev.SoliditySignature, req.BatchId)
		var lockDelegated NotoLockDelegated_Event
		if err := json.Unmarshal([]byte(ev.DataJson), &lockDelegated); err == nil {
			txData, err := n.decodeTransactionDataV1(ctx, lockDelegated.Data)
			if err != nil {
				return err
			}
			n.recordTransactionInfo(ev, lockDelegated.TxId, txData.InfoStates, res)
		} else {
			log.L(ctx).Warnf("Ignoring malformed NotoLockDelegated event in batch %s: %s", req.BatchId, err)
		}
	}

	// Handle new states representing new SMT nodes
	if useNullifier {
		newStatesForSMT, err := smtForStates.Storage.GetNewStates()
		if err != nil {
			log.L(ctx).Errorf("Failed to get new SMT states for tree %s: %s", smtForStates.Name, err)
			return nil
		}
		if len(newStatesForSMT) > 0 {
			res.NewStates = append(res.NewStates, newStatesForSMT...)
		}
	}

	return nil
}

// When notary logic is implemented via Pente, unlock events from the base ledger must be propagated back to the Pente hooks
// TODO: this method should not be invoked directly on the event loop, but rather via a queue
func (n *Noto) handleNotaryPrivateUnlock(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, lockedInputs []pldtypes.Bytes32, outputs []pldtypes.Bytes32, sender *pldtypes.EthAddress, data pldtypes.HexBytes, lockID pldtypes.Bytes32) error {
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
			Sender:     sender,
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

func (n *Noto) handleNotaryPrivateUnlockV1(ctx context.Context, stateQueryContext string, domainConfig *types.NotoParsedConfig, unlock *NotoUnlock_Event) error {
	// V1: lockId is in the event
	return n.handleNotaryPrivateUnlock(ctx, stateQueryContext, domainConfig, unlock.LockedInputs, unlock.Outputs, unlock.Sender, unlock.Data, unlock.LockId)
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

func (n *Noto) updateMerkleTree(ctx context.Context, tree core.SparseMerkleTree, storage smt.StatesStorage, txID pldtypes.Bytes32, outputs []pldtypes.HexUint256) error {
	storage.SetTransactionId(txID.HexString0xPrefix())
	for _, out := range outputs {
		if out.NilOrZero() {
			continue
		}
		err := n.addOutputToMerkleTree(ctx, tree, out)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *Noto) addOutputToMerkleTree(ctx context.Context, tree core.SparseMerkleTree, output pldtypes.HexUint256) error {
	idx, err := node.NewNodeIndexFromBigInt(output.Int(), notosmt.GetHasher())
	if err != nil {
		return i18n.NewError(ctx, pldmsgs.MsgErrorNewNodeIndex, output.String(), err)
	}
	nidx := node.NewIndexOnly(idx)
	leaf, err := node.NewLeafNode(nidx, nil)
	if err != nil {
		return i18n.NewError(ctx, pldmsgs.MsgErrorNewLeafNode, err)
	}
	err = tree.AddLeaf(leaf)
	if err != nil {
		return i18n.NewError(ctx, pldmsgs.MsgErrorAddLeafNode, err)
	}
	return nil
}

func convertToUint256(in []pldtypes.Bytes32) []pldtypes.HexUint256 {
	out := make([]pldtypes.HexUint256, len(in))
	for i, v := range in {
		out[i] = *pldtypes.MustParseHexUint256(v.String())
	}
	return out
}
