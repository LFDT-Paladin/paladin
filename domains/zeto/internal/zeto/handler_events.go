// Event indexing (HandleEventBatch): legacy zeto-contracts ~v0.2.x pools emit UTXOsLocked and UTXOTransferWithEncryptedValues.
// v0.5-style pools emit ZetoLockCreated / ZetoLockSpent / ZetoLockCancelled. UTXOTransferWithMlkemEncryptedValues is omitted until Paladin ships a token that uses ML-KEM transfers (see IZeto_V1 ABI).
//
// Nullifier (SMT) semantics: legacy UTXOsLocked records unlocked outputs in the main state SMT and locked outputs in the locked SMT.
// For v0.5 ZetoLock* events, only unlocked outputs feed the nullifier SMT; locked outputs are tracked as locked UTXOs off-tree (v0.5 spendLock does not use nullifier proofs over locked inputs). Locked inputs on spend/cancel events are not inserted into either SMT here.
//
// Spent and confirmed state IDs are derived from uint256 UTXO roots in events plus Paladin ZetoTransactionData_V0 in the data payload.
// Generic LockCreated (bytes32 lockId) registry events are not UTXO state events and are ignored here.
//
// Token implementations exercised in CI-style integration coverage are listed in domains/integration-test/zeto_fungible_test.go
// (Zeto_Anon, Zeto_AnonEnc, Zeto_AnonNullifier, Zeto_AnonNullifierKyc and batch variants) and domains/integration-test/zeto_nonfungible_test.go (Zeto_NfAnon).

package zeto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	signercommon "github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/signer/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/smt"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/core"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/node"
	"github.com/iden3/go-iden3-crypto/poseidon"
)

func (z *Zeto) recordTransactionInfo(ev *prototk.OnChainEvent, txData *types.ZetoTransactionData_V0, res *prototk.HandleEventBatchResponse) {
	res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
		TransactionId: txData.TransactionID.String(),
		Location:      ev.Location,
	})
	for _, state := range txData.InfoStates {
		res.InfoStates = append(res.InfoStates, &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txData.TransactionID.String(),
		})
	}
}

func (z *Zeto) handleMintEvent(ctx context.Context, smtTree *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var mint MintEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &mint); err == nil {
		txData, err := decodeTransactionData(ctx, mint.Data)
		if err != nil || txData == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for mint event: %s. Skip to the next event", mint.Data)
			return nil
		}
		z.recordTransactionInfo(ev, txData, res)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, mint.Outputs)...)
		if common.IsNullifiersToken(tokenName) {
			err := z.updateMerkleTree(ctx, smtTree.Tree, smtTree.Storage, txData.TransactionID, mint.Outputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOMint", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal mint event: %s", err)
	}
	return nil
}

func (z *Zeto) handleTransferEvent(ctx context.Context, smtTree *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var transfer TransferEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
		txData, err := decodeTransactionData(ctx, transfer.Data)
		if err != nil || txData == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for transfer event: %s. Skip to the next event", transfer.Data)
			return nil
		}
		z.recordTransactionInfo(ev, txData, res)
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txData.TransactionID, transfer.Inputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, transfer.Outputs)...)
		if common.IsNullifiersToken(tokenName) {
			err := z.updateMerkleTree(ctx, smtTree.Tree, smtTree.Storage, txData.TransactionID, transfer.Outputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOTransfer", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal transfer event: %s", err)
	}
	return nil
}

func (z *Zeto) handleTransferWithEncryptionEvent(ctx context.Context, smtTree *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var transfer TransferWithEncryptedValuesEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
		txData, err := decodeTransactionData(ctx, transfer.Data)
		if err != nil || txData == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for transfer event: %s. Skip to the next event", transfer.Data)
			return nil
		}
		z.recordTransactionInfo(ev, txData, res)
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txData.TransactionID, transfer.Inputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, transfer.Outputs)...)
		if common.IsNullifiersToken(tokenName) {
			err := z.updateMerkleTree(ctx, smtTree.Tree, smtTree.Storage, txData.TransactionID, transfer.Outputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOTransferWithEncryptedValues", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal transfer event: %s", err)
	}
	return nil
}

func (z *Zeto) handleWithdrawEvent(ctx context.Context, smtTree *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var withdraw WithdrawEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &withdraw); err == nil {
		txData, err := decodeTransactionData(ctx, withdraw.Data)
		if err != nil || txData == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for withdraw event: %s. Skip to the next event", withdraw.Data)
			return nil
		}
		z.recordTransactionInfo(ev, txData, res)
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txData.TransactionID, withdraw.Inputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, []pldtypes.HexUint256{withdraw.Output})...)
		if common.IsNullifiersToken(tokenName) {
			err := z.updateMerkleTree(ctx, smtTree.Tree, smtTree.Storage, txData.TransactionID, []pldtypes.HexUint256{withdraw.Output})
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOWithdraw", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal withdraw event: %s", err)
	}
	return nil
}

// handleLockedEvent indexes legacy UTXOsLocked (~v0.2.x): nullifier tokens write unlocked outputs to the main SMT and locked outputs to the locked SMT.
func (z *Zeto) handleLockedEvent(ctx context.Context, smtTree *common.MerkleTreeSpec, smtTreeForLocked *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var lock LockedEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &lock); err == nil {
		txData, err := decodeTransactionData(ctx, lock.Data)
		if err != nil || txData == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for lock event: %s. Skip to the next event", lock.Data)
			return nil
		}
		z.recordTransactionInfo(ev, txData, res)
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txData.TransactionID, lock.Inputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, lock.Outputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, lock.LockedOutputs)...)
		if common.IsNullifiersToken(tokenName) {
			err := z.updateMerkleTree(ctx, smtTree.Tree, smtTree.Storage, txData.TransactionID, lock.Outputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOsLocked", err)
			}
			err = z.updateMerkleTree(ctx, smtTreeForLocked.Tree, smtTreeForLocked.Storage, txData.TransactionID, lock.LockedOutputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "UTXOsLocked", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal lock event: %s", err)
	}
	return nil
}

// handleZetoLockCreatedEvent indexes ILockableCapability v0.5 lock creation.
// Unlocked outputs participate in the nullifier SMT for nullifier tokens; locked outputs do not (they are locked UTXOs, not SMT leaves).
func (z *Zeto) handleZetoLockCreatedEvent(ctx context.Context, stateQueryContext string, smtTree *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var lock ZetoLockCreatedEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &lock); err == nil {
		txData, err := decodeTransactionData(ctx, lock.Data)
		if err != nil || txData == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for lock event: %s. Skip to the next event", lock.Data)
			return nil
		}
		z.recordTransactionInfo(ev, txData, res)
		log.L(ctx).Infof("ZetoLockCreated: chainLockId=%s chainOwner=%s paladinTxId=%s (lock-info state id equals chain lockId)",
			lock.LockId.HexString0xPrefix(), lock.Owner.String(), txData.TransactionID.HexString0xPrefix())
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txData.TransactionID, lock.Inputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, lock.Outputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, lock.LockedOutputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStateUpdatesFromBytes32(txData.TransactionID, []pldtypes.Bytes32{lock.LockId})...)
		if common.IsNullifiersToken(tokenName) {
			// the locked outputs are not added to the nullifier SMT, because they are not spent with nullifiers
			// but spent directly with their commitment IDs
			err := z.updateMerkleTree(ctx, smtTree.Tree, smtTree.Storage, txData.TransactionID, lock.Outputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "ZetoLockCreated", err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal ZetoLockCreated event: %s", err)
	}
	return nil
}

type lockInfoRow struct {
	stateID string
	info    *types.ZetoLockInfoState
}

func (z *Zeto) loadLockInfoRowByLockID(ctx context.Context, stateQueryContext string, lockID pldtypes.Bytes32) (*lockInfoRow, error) {
	if z.lockInfoSchema == nil || lockID.IsZero() {
		return nil, nil
	}
	queryJSON := query.NewQueryBuilder().Limit(1).Equal("lockId", lockID).Query().String()
	res, err := z.Callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
		StateQueryContext: stateQueryContext,
		SchemaId:          z.lockInfoSchema.Id,
		QueryJson:         queryJSON,
	})
	if err != nil {
		return nil, err
	}
	if res == nil || len(res.States) == 0 {
		return nil, nil
	}
	st := res.States[0]
	var li types.ZetoLockInfoState
	if err := json.Unmarshal([]byte(st.DataJson), &li); err != nil {
		return nil, err
	}
	return &lockInfoRow{stateID: st.Id, info: &li}, nil
}

// spendLockInfoStateForLock consumes the off-chain lock-info row when a lock is spent or cancelled (Noto OldLockState).
func (z *Zeto) spendLockInfoStateForLock(ctx context.Context, stateQueryContext string, lockID, txID pldtypes.Bytes32, res *prototk.HandleEventBatchResponse) error {
	row, err := z.loadLockInfoRowByLockID(ctx, stateQueryContext, lockID)
	if err != nil || row == nil {
		return err
	}
	res.SpentStates = append(res.SpentStates, &prototk.StateUpdate{
		Id:            row.stateID,
		TransactionId: txID.String(),
	})
	return nil
}

// confirmProposedSpendOutputsForLock makes pre-pinned spend coin InfoStates available to recipients when a lock is spent.
// spendLock proof outputs are the pinned commitments (payload.Outputs); this confirms the Paladin state rows by id.
// They are created at createLock assemble but must not confirm until ZetoLockSpent (not ZetoLockCreated).
func (z *Zeto) confirmProposedSpendOutputsForLock(ctx context.Context, stateQueryContext string, lockID, txID pldtypes.Bytes32, res *prototk.HandleEventBatchResponse) error {
	if z.lockInfoSchema == nil || lockID.IsZero() {
		return nil
	}
	row, err := z.loadLockInfoRowByLockID(ctx, stateQueryContext, lockID)
	if err != nil {
		return err
	}
	if row == nil {
		return nil
	}
	txIDStr := txID.String()
	for _, out := range row.info.SpendOutputs {
		coinID, err := common.CoinStateIDFromPersistedString(ctx, out)
		if err != nil {
			log.L(ctx).Warnf("Skipping invalid spendOutputs entry %q for lock %s: %s", out, lockID.HexString0xPrefix(), err)
			continue
		}
		res.ConfirmedStates = append(res.ConfirmedStates, &prototk.StateUpdate{
			Id:            coinID,
			TransactionId: txIDStr,
		})
	}
	return nil
}

// handleZetoLockSpentLikeEvent indexes ZetoLockSpent / ZetoLockCancelled.
// Only unlocked outputs are added to the nullifier SMT; locked inputs/outputs use direct locked-UTXO semantics (no locked SMT updates here).
func (z *Zeto) handleZetoLockSpentLikeEvent(ctx context.Context, stateQueryContext string, smtTree *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse, eventName string) error {
	var payload ZetoLockSpentLikeEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &payload); err == nil {
		txData, decodeErr := decodeTransactionData(ctx, payload.Data)
		if decodeErr != nil {
			log.L(ctx).Errorf("Failed to decode transaction data for %s event: %s", eventName, decodeErr)
		}
		txID := payload.TxId
		if txData != nil {
			txID = txData.TransactionID
		} else if txID.IsZero() {
			log.L(ctx).Errorf("Failed to decode transaction data for %s event and event txId is zero: data=%s. Skip to the next event", eventName, payload.Data)
			return nil
		}
		if txData != nil {
			z.recordTransactionInfo(ev, txData, res)
		} else {
			// Outer `data` may be empty or not yet V0-encoded on some public legs; still finalize the spend
			// so prepared states reconcile and nullifier SMT ingests unlocked outputs (v0.5 spendLock unlock).
			res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
				TransactionId: txID.String(),
				Location:      ev.Location,
			})
		}
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txID, payload.LockedInputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txID, payload.Outputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txID, payload.LockedOutputs)...)
		if err := z.spendLockInfoStateForLock(ctx, stateQueryContext, payload.LockId, txID, res); err != nil {
			return err
		}
		if eventName == "ZetoLockSpent" {
			if err := z.confirmProposedSpendOutputsForLock(ctx, stateQueryContext, payload.LockId, txID, res); err != nil {
				return err
			}
		}
		if common.IsNullifiersToken(tokenName) {
			// the locked outputs are not added to the nullifier SMT, because they are not spent with nullifiers
			// but spent directly with their commitment IDs
			err := z.updateMerkleTree(ctx, smtTree.Tree, smtTree.Storage, txID, payload.Outputs)
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, eventName, err)
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal %s event: %s", eventName, err)
	}
	return nil
}

func (z *Zeto) handleIdentityRegisteredEvent(ctx context.Context, smtKycTree *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var registered IdentityRegisteredEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &registered); err == nil {
		txData, err := decodeTransactionData(ctx, registered.Data)
		if err != nil || txData == nil {
			txId := pldtypes.MustParseBytes32("0000000000000000000000000000000000000000000000000000000000000000")
			log.L(ctx).Infof("IdentityRegistered event [publicKey=%+v] has no tx data. Inserting zero tx ID", &registered.PublicKey)
			txData = &types.ZetoTransactionData_V0{
				TransactionID: txId,
				InfoStates:    []pldtypes.Bytes32{},
			}
		}
		if common.IsKycToken(tokenName) {
			// calculate the Poseidon hash of the public key, to use as the leaf node index and value in the SMT
			publicKeyHash, err := poseidon.Hash([]*big.Int{registered.PublicKey[0].Int(), registered.PublicKey[1].Int()})
			if err != nil {
				return i18n.NewError(ctx, msgs.MsgErrorHandleEvents, fmt.Sprintf("IdentityRegistered. %s", err))
			}
			err = z.updateMerkleTree(ctx, smtKycTree.Tree, smtKycTree.Storage, txData.TransactionID, []pldtypes.HexUint256{*pldtypes.MustParseHexUint256("0x" + publicKeyHash.Text(16))})
			if err != nil {
				// Check if this is a "key already exists" error - if so, treat it as success for idempotency
				if strings.Contains(err.Error(), "key already exists") || strings.Contains(err.Error(), "already exists") {
					log.L(ctx).Infof("Identity with publicKey=%+v is already registered in KYC tree - treating as success", &registered.PublicKey)
				} else {
					return i18n.NewError(ctx, msgs.MsgErrorUpdateSMT, "IdentityRegistered", err)
				}
			}
		}
	} else {
		log.L(ctx).Errorf("Failed to unmarshal IdentityRegistered event: %s", err)
	}
	return nil
}

func (z *Zeto) updateMerkleTree(ctx context.Context, tree core.SparseMerkleTree, storage smt.StatesStorage, txID pldtypes.Bytes32, outputs []pldtypes.HexUint256) error {
	storage.SetTransactionId(txID.HexString0xPrefix())
	for _, out := range outputs {
		if out.NilOrZero() {
			continue
		}
		err := z.addOutputToMerkleTree(ctx, tree, out)
		if err != nil {
			return err
		}
	}
	return nil
}

func (z *Zeto) addOutputToMerkleTree(ctx context.Context, tree core.SparseMerkleTree, output pldtypes.HexUint256) error {
	idx, err := node.NewNodeIndexFromBigInt(output.Int(), signercommon.GetHasher())
	if err != nil {
		return i18n.NewError(ctx, pldmsgs.MsgErrorNewNodeIndex, output.String(), err)
	}
	n := node.NewIndexOnly(idx)
	leaf, err := node.NewLeafNode(n, nil)
	if err != nil {
		return i18n.NewError(ctx, pldmsgs.MsgErrorNewLeafNode, err)
	}
	err = tree.AddLeaf(ctx, leaf)
	if err != nil {
		return i18n.NewError(ctx, pldmsgs.MsgErrorAddLeafNode, err)
	}
	return nil
}

func parseStatesFromEvent(txID pldtypes.Bytes32, states []pldtypes.HexUint256) []*prototk.StateUpdate {
	refs := make([]*prototk.StateUpdate, len(states))
	for i, state := range states {
		refs[i] = &prototk.StateUpdate{
			Id:            common.HexUint256To32ByteHexString(&state),
			TransactionId: txID.String(),
		}
	}
	return refs
}

func parseStateUpdatesFromBytes32(txID pldtypes.Bytes32, states []pldtypes.Bytes32) []*prototk.StateUpdate {
	refs := make([]*prototk.StateUpdate, len(states))
	for i, state := range states {
		refs[i] = &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txID.String(),
		}
	}
	return refs
}

func formatErrors(errors []string) string {
	msg := fmt.Sprintf("(failures=%d)", len(errors))
	for i, err := range errors {
		msg = fmt.Sprintf("%s. [%d]%s", msg, i, err)
	}
	return msg
}

// sliceZetoTransactionDataPayload returns the sub-slice of `data` that begins with ZetoTransactionDataID_V0
// (Paladin ZetoTransactionData_V0 envelope). ILockableCapability v0.5 lock/spend outer `data` bytes prepend
// unlock preimage (e.g. 32-byte delegate) before this envelope, so the magic is not always at offset 0.
func sliceZetoTransactionDataPayload(data pldtypes.HexBytes) pldtypes.HexBytes {
	if len(data) < 4 {
		return nil
	}
	magic := []byte(types.ZetoTransactionDataID_V0)
	if len(data) >= len(magic) && bytes.HasPrefix(data, magic) {
		return data
	}
	idx := bytes.Index(data, magic)
	if idx < 0 {
		return nil
	}
	return data[idx:]
}

func decodeTransactionData(ctx context.Context, data pldtypes.HexBytes) (*types.ZetoTransactionData_V0, error) {
	payload := sliceZetoTransactionDataPayload(data)
	if len(payload) < 4 {
		return nil, nil
	}

	var dataValues types.ZetoTransactionData_V0
	dataDecoded, err := types.ZetoTransactionDataABI_V0.DecodeABIDataCtx(ctx, payload, 4)
	if err == nil {
		var dataJSON []byte
		dataJSON, err = dataDecoded.JSON()
		if err == nil {
			err = json.Unmarshal(dataJSON, &dataValues)
		}
	}
	if err != nil {
		return nil, err
	}
	return &dataValues, nil
}
