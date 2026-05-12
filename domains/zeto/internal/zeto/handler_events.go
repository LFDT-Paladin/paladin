// Event indexing (HandleEventBatch): legacy zeto-contracts ~v0.2.x pools emit UTXOsLocked and UTXOTransferWithEncryptedValues.
// v0.5-style pools emit ZetoLockCreated / ZetoLockSpent / ZetoLockCancelled. UTXOTransferWithMlkemEncryptedValues is omitted until Paladin ships a token that uses ML-KEM transfers (see IZeto_V1 ABI).
//
// Nullifier (SMT) semantics: legacy UTXOsLocked records unlocked outputs in the main state SMT and locked outputs in a separate locked SMT.
// For v0.5 ZetoLock* events, only unlocked outputs feed the nullifier SMT; locked outputs are tracked as locked UTXOs off-tree (spent directly without nullifiers). Locked inputs on spend/cancel events are not inserted into either SMT here.
//
// Spent and confirmed state IDs are derived from uint256 UTXO roots in events plus Paladin ZetoTransactionData_V0 in the data payload.
// Generic LockCreated (bytes32 lockId) registry events are not UTXO state events and are ignored here.
//
// Token implementations exercised in CI-style integration coverage are listed in domains/integration-test/zeto_fungible_test.go
// (Zeto_Anon, Zeto_AnonEnc, Zeto_AnonNullifier, Zeto_AnonNullifierKyc and batch variants) and domains/integration-test/zeto_nonfungible_test.go (Zeto_NfAnon).

package zeto

import (
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
func (z *Zeto) handleZetoLockCreatedEvent(ctx context.Context, smtTree *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse) error {
	var lock ZetoLockCreatedEvent
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

// handleZetoLockSpentLikeEvent indexes ZetoLockSpent / ZetoLockCancelled.
// Only unlocked outputs are added to the nullifier SMT; locked inputs/outputs use direct locked-UTXO semantics (no locked SMT updates here).
func (z *Zeto) handleZetoLockSpentLikeEvent(ctx context.Context, smtTree *common.MerkleTreeSpec, ev *prototk.OnChainEvent, tokenName string, res *prototk.HandleEventBatchResponse, eventName string) error {
	var payload ZetoLockSpentLikeEvent
	if err := json.Unmarshal([]byte(ev.DataJson), &payload); err == nil {
		txData, err := decodeTransactionData(ctx, payload.Data)
		if err != nil || txData == nil {
			log.L(ctx).Errorf("Failed to decode transaction data for %s event: %s. Skip to the next event", eventName, payload.Data)
			return nil
		}
		z.recordTransactionInfo(ev, txData, res)
		res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txData.TransactionID, payload.LockedInputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, payload.Outputs)...)
		res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txData.TransactionID, payload.LockedOutputs)...)
		if common.IsNullifiersToken(tokenName) {
			// the locked outputs are not added to the nullifier SMT, because they are not spent with nullifiers
			// but spent directly with their commitment IDs
			err := z.updateMerkleTree(ctx, smtTree.Tree, smtTree.Storage, txData.TransactionID, payload.Outputs)
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

func formatErrors(errors []string) string {
	msg := fmt.Sprintf("(failures=%d)", len(errors))
	for i, err := range errors {
		msg = fmt.Sprintf("%s. [%d]%s", msg, i, err)
	}
	return msg
}

func decodeTransactionData(ctx context.Context, data pldtypes.HexBytes) (*types.ZetoTransactionData_V0, error) {
	if len(data) < 4 {
		return nil, nil
	}
	dataPrefix := data[0:4]
	if dataPrefix.String() != types.ZetoTransactionDataID_V0.String() {
		return nil, nil
	}

	var dataValues types.ZetoTransactionData_V0
	dataDecoded, err := types.ZetoTransactionDataABI_V0.DecodeABIDataCtx(ctx, data, 4)
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
