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

package fungible

import (
	"context"
	"encoding/json"
	"math/big"
	"slices"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	zetosmt "github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/smt"
	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/smt"
	utxocore "github.com/LFDT-Paladin/smt/pkg/utxo/core"

	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/core"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/node"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"google.golang.org/protobuf/proto"
)

// due to the ZKP circuit needing to check if the amount is positive,
// the maximum transfer amount is (2^100 - 1)
// Reference: https://github.com/hyperledger-labs/zeto/blob/main/zkp/circuits/lib/check-positive.circom
var MAX_TRANSFER_AMOUNT = big.NewInt(0).Exp(big.NewInt(2), big.NewInt(100), nil)

type baseHandler struct {
	name         string
	stateSchemas *common.StateSchemas
}

func (h *baseHandler) getAlgoZetoSnarkBJJ() string {
	return getAlgoZetoSnarkBJJ(h.name)
}

func validateTransferParams(ctx context.Context, params []*types.FungibleTransferParamEntry) error {
	if len(params) == 0 {
		return i18n.NewError(ctx, msgs.MsgNoTransferParams)
	}
	total := big.NewInt(0)
	for i, param := range params {
		if param.To == "" {
			return i18n.NewError(ctx, msgs.MsgNoParamTo, i)
		}
		if err := validateAmountParam(ctx, param.Amount, i); err != nil {
			return err
		}
		total.Add(total, param.Amount.Int())
	}
	if total.Cmp(MAX_TRANSFER_AMOUNT) >= 0 {
		return i18n.NewError(ctx, msgs.MsgParamTotalAmountInRange)
	}

	return nil
}

func validateBalanceOfParams(ctx context.Context, param *types.FungibleBalanceOfParam) error {
	if param.Account == "" {
		return i18n.NewError(ctx, msgs.MsgNoParamAccount)
	}
	return nil
}

func validateAmountParam(ctx context.Context, amount *pldtypes.HexUint256, i int) error {
	if amount == nil {
		return i18n.NewError(ctx, msgs.MsgNoParamAmount, i)
	}
	if amount.Int().Sign() != 1 {
		return i18n.NewError(ctx, msgs.MsgParamAmountInRange, i)
	}
	return nil
}

func utxosFromInputStates(ctx context.Context, states []*prototk.EndorsableState, desiredSize int) ([]string, error) {
	return utxosFromStates(ctx, states, desiredSize, true)
}

func utxosFromOutputStates(ctx context.Context, states []*prototk.EndorsableState, desiredSize int) ([]string, error) {
	return utxosFromStates(ctx, states, desiredSize, false)
}

// utxosFromCoins builds on-chain output commitment strings from coin rows (e.g. createLock spend-pinned outputs).
func utxosFromCoins(ctx context.Context, coins []*types.ZetoCoin, desiredSize int) ([]string, error) {
	utxos := make([]string, desiredSize)
	for i := 0; i < desiredSize; i++ {
		if i < len(coins) {
			hash, err := coins[i].Hash(ctx)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorParseOutputStates, err)
			}
			utxos[i] = hash.String()
		} else {
			utxos[i] = "0"
		}
	}
	return utxos, nil
}

func utxosFromStates(ctx context.Context, states []*prototk.EndorsableState, desiredSize int, isInputs bool) ([]string, error) {
	utxos := make([]string, desiredSize)
	for i := 0; i < desiredSize; i++ {
		if i < len(states) {
			msgTemplate := msgs.MsgErrorParseInputStates
			if !isInputs {
				msgTemplate = msgs.MsgErrorParseOutputStates
			}
			state := states[i]
			coin, err := makeCoin(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgTemplate, err)
			}
			hash, err := coin.Hash(ctx)
			if err != nil {
				return nil, i18n.NewError(ctx, msgTemplate, err)
			}
			utxos[i] = hash.String()
		} else {
			utxos[i] = "0"
		}
	}
	return utxos, nil
}

func generateMerkleProofs(ctx context.Context, smtSpec *common.MerkleTreeSpec, indexes []*big.Int, targetSize int) (*corepb.MerkleProofObject, error) {
	// verify that the input UTXOs have been indexed by the Merkle tree DB
	// and generate a merkle proof for each
	mtRoot := smtSpec.Tree.Root()
	proofs, _, err := smtSpec.Tree.GenerateProofs(ctx, indexes, mtRoot)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
	}
	var mps []*corepb.MerkleProof
	var enabled []bool
	for i, proof := range proofs {
		cp, err := proof.ToCircomVerifierProof(indexes[i], indexes[i], mtRoot, smtSpec.Levels)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorConvertToCircomProof, err)
		}
		proofSiblings := make([]string, len(cp.Siblings)-1)
		for i, s := range cp.Siblings[0 : len(cp.Siblings)-1] {
			proofSiblings[i] = s.BigInt().Text(16)
		}
		p := corepb.MerkleProof{
			Nodes: proofSiblings,
		}
		mps = append(mps, &p)
		enabled = append(enabled, true)
	}
	// if the proofs are less than the target size, we need to fill the rest with empty proofs
	size := len(mps)
	for i := size; i < targetSize; i++ {
		mps = append(mps, smtSpec.EmptyProof)
		enabled = append(enabled, false)
	}
	smtProof := &corepb.MerkleProofObject{
		Root:         mtRoot.BigInt().Text(16),
		MerkleProofs: mps,
		Enabled:      enabled,
	}

	return smtProof, nil
}

// paddedDisabledMerkleProof builds a fixed-size proof object with every slot disabled. v0.5 spendLock on
// nullifier+KYC tokens spends locked inputs by commitment (lockVerifier / transferLocked circuit) without
// nullifier SMT membership for those inputs.
func paddedDisabledMerkleProof(ctx context.Context, smtSpec *common.MerkleTreeSpec, targetSize int) (*corepb.MerkleProofObject, error) {
	mtRoot := smtSpec.Tree.Root()
	mps := make([]*corepb.MerkleProof, targetSize)
	enabled := make([]bool, targetSize)
	for i := 0; i < targetSize; i++ {
		mps[i] = smtSpec.EmptyProof
		enabled[i] = false
	}
	return &corepb.MerkleProofObject{
		Root:         mtRoot.BigInt().Text(16),
		MerkleProofs: mps,
		Enabled:      enabled,
	}, nil
}

func makeLeafIndexesFromCoins(ctx context.Context, inputCoins []*types.ZetoCoin, mt core.SparseMerkleTree, hasher utxocore.Hasher) ([]*big.Int, error) {
	var indexes []*big.Int
	for _, coin := range inputCoins {
		pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(coin.Owner.String())
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}
		// Create a new fungible node for the coin, to check existence
		// in the Merkle tree. The index is calculated from the coin's
		// amount, owner and salt.
		idx := node.NewFungible(coin.Amount.Int(), pubKey, coin.Salt.Int(), hasher)
		leaf, err := node.NewLeafNode(idx, nil)
		if err != nil {
			return nil, i18n.NewError(ctx, pldmsgs.MsgErrorNewLeafNode, err)
		}
		// Check if the leaf exists in the Merkle tree
		n, err := mt.GetNode(ctx, leaf.Ref())
		if err != nil {
			// TODO: deal with when the node is not found in the DB tables for the tree
			// e.g because the transaction event hasn't been processed yet
			return nil, i18n.NewError(ctx, pldmsgs.MsgErrorQueryLeafNode, leaf.Ref().Hex(), err)
		}
		// Check if the index of the node returned from the merkle tree
		// matches the expected index calculated from the coin's amount, owner and salt.
		hash, err := coin.Hash(ctx)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
		}
		if n.Index().BigInt().Cmp(hash.Int()) != 0 {
			expectedIndex, err := node.NewNodeIndexFromBigInt(hash.Int(), hasher)
			if err != nil {
				return nil, i18n.NewError(ctx, pldmsgs.MsgErrorNewNodeIndex, err)
			}
			// we have found a node in the tree based on its primary key (ref),
			// but the node's index doesn't match the expected index based on
			// the coin's amount, owner and salt. This is an error situation
			return nil, i18n.NewError(ctx, msgs.MsgErrorHashMismatch, leaf.Ref().Hex(), n.Index().BigInt().Text(16), n.Index().Hex(), hash.HexString0xPrefix(), expectedIndex.Hex())
		}
		indexes = append(indexes, n.Index().BigInt())
	}
	return indexes, nil
}

func makeLeafIndexesFromCoinOwners(ctx context.Context, inputOwner string, outputCoins []*types.ZetoCoin) ([]*big.Int, error) {
	indexes := make([]*big.Int, len(outputCoins)+1) // +1 for the input owner
	// the first index is for the input owner
	pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(inputOwner)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
	}
	hash, err := poseidon.Hash([]*big.Int{pubKey.X, pubKey.Y})
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
	}
	indexes[0] = hash
	// the rest of the indexes are for the output coins
	for i, coin := range outputCoins {
		pubKey, err := zetosigner.DecodeBabyJubJubPublicKey(coin.Owner.String())
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}
		hash, err := poseidon.Hash([]*big.Int{pubKey.X, pubKey.Y})
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorHashOutputState, err)
		}
		indexes[i+1] = hash
	}
	return indexes, nil
}

// formatTransferProvingRequest formats the proving request for a fungible transfer, lock (legacy), createLock,
// transferLocked, or spendLock — use the domain circuit for the operation (e.g. transfer vs transferLocked).
// smtFromLockedStatesTree selects the SMT used for nullifier Merkle proofs when UsesNullifiers is true:
// false = unlocked UTXO tree (transfer, createLock, legacy lock, v0.5 spendLock), true = locked-output tree
// (transferLocked with nullifier-based locked inputs). Optional delegate sets lockDelegate in nullifier extras
// only when the target circuit expects it (not anon_nullifier_transfer createLock — see zeto_anon_nullifier.ts).
func formatTransferProvingRequest(ctx context.Context, callbacks plugintk.DomainCallbacks, merkleTreeRootSchema *prototk.StateSchema, merkleTreeNodeSchema *prototk.StateSchema, hasher utxocore.Hasher, inputCoins, outputCoins []*types.ZetoCoin, circuit *zetosignerapi.Circuit, tokenName, stateQueryContext string, contractAddress *pldtypes.EthAddress, smtFromLockedStatesTree bool, delegate ...string) ([]byte, error) {
	inputSize := common.GetInputSize(len(inputCoins))
	inputCommitments := make([]string, inputSize)
	inputValueInts := make([]uint64, inputSize)
	inputSalts := make([]string, inputSize)
	inputOwner := inputCoins[0].Owner.String()
	for i := 0; i < inputSize; i++ {
		if i < len(inputCoins) {
			coin := inputCoins[i]
			hash, err := coin.Hash(ctx)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
			}
			inputCommitments[i] = hash.Int().Text(16)
			inputValueInts[i] = coin.Amount.Int().Uint64()
			inputSalts[i] = coin.Salt.Int().Text(16)
		} else {
			inputCommitments[i] = "0"
			inputSalts[i] = "0"
		}
	}

	outputValueInts := make([]uint64, inputSize)
	outputSalts := make([]string, inputSize)
	outputOwners := make([]string, inputSize)
	for i := 0; i < inputSize; i++ {
		if i < len(outputCoins) {
			coin := outputCoins[i]
			outputValueInts[i] = coin.Amount.Int().Uint64()
			outputSalts[i] = coin.Salt.Int().Text(16)
			outputOwners[i] = coin.Owner.String()
		} else {
			outputSalts[i] = "0"
			// Match zeto-js prepareProof + inflateOwners: padded output slots reuse the first
			// output owner's BabyJub key (not an empty string), so the witness matches on-chain padding.
			outputOwners[i] = outputOwners[0]
		}
	}

	var extras []byte
	if circuit.UsesNullifiers {
		smtProof, err := smtProofForInputs(ctx, callbacks, merkleTreeRootSchema, merkleTreeNodeSchema, hasher, tokenName, stateQueryContext, contractAddress, inputCoins, smtFromLockedStatesTree, inputSize)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
		}

		var extrasObj proto.Message
		if !circuit.UsesKyc {
			extrasObj = &corepb.ProvingRequestExtras_Nullifiers{
				SmtProof: smtProof,
			}
			if len(delegate) > 0 {
				extrasObj.(*corepb.ProvingRequestExtras_Nullifiers).Delegate = delegate[0]
			}
		} else {
			// for KYC, we need the additional proof for the KYC states
			smtProofKyc, err := smtProofForOwners(ctx, callbacks, merkleTreeRootSchema, merkleTreeNodeSchema, tokenName, stateQueryContext, contractAddress, inputOwner, outputCoins, inputSize+1)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
			}

			extrasObj = &corepb.ProvingRequestExtras_NullifiersKyc{
				SmtUtxoProof: smtProof,
				SmtKycProof:  smtProofKyc,
			}
			if len(delegate) > 0 {
				extrasObj.(*corepb.ProvingRequestExtras_NullifiersKyc).Delegate = delegate[0]
			}
		}
		protoExtras, err := proto.Marshal(extrasObj)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalExtraObj, err)
		}
		extras = protoExtras
	} else if circuit.UsesKyc && !circuit.UsesNullifiers {
		smtName := zetosmt.MerkleTreeName(tokenName, contractAddress)
		mt, err := common.NewMerkleTreeSpec(ctx, smtName, smt.StatesTree, callbacks, merkleTreeRootSchema.Id, merkleTreeNodeSchema.Id, stateQueryContext)
		if err != nil {
			return nil, err
		}
		smtProofUtxo, err := paddedDisabledMerkleProof(ctx, mt, inputSize)
		if err != nil {
			return nil, err
		}
		smtProofKyc, err := smtProofForOwners(ctx, callbacks, merkleTreeRootSchema, merkleTreeNodeSchema, tokenName, stateQueryContext, contractAddress, inputOwner, outputCoins, inputSize+1)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
		}
		extrasObj := &corepb.ProvingRequestExtras_NullifiersKyc{
			SmtUtxoProof: smtProofUtxo,
			SmtKycProof:  smtProofKyc,
		}
		if len(delegate) > 0 {
			extrasObj.Delegate = delegate[0]
		}
		protoExtras, err := proto.Marshal(extrasObj)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalExtraObj, err)
		}
		extras = protoExtras
	}
	tokenSecrets, err := marshalTokenSecrets(inputValueInts, outputValueInts)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalValuesFungible, err)
	}
	payload := &corepb.ProvingRequest{
		Circuit: circuit.ToProto(),
		Common: &corepb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputSalts:       inputSalts,
			InputOwner:       inputOwner,
			OutputSalts:      outputSalts,
			OutputOwners:     outputOwners,
			TokenSecrets:     tokenSecrets,
			TokenType:        corepb.TokenType_fungible,
		},
	}
	if extras != nil {
		payload.Extras = extras
	}
	return proto.Marshal(payload)
}

func smtProofForInputs(ctx context.Context, callbacks plugintk.DomainCallbacks, merkleTreeRootSchema *prototk.StateSchema, merkleTreeNodeSchema *prototk.StateSchema, hasher utxocore.Hasher, tokenName, stateQueryContext string, contractAddress *pldtypes.EthAddress, inputCoins []*types.ZetoCoin, forLockedStates bool, targetSize int) (*corepb.MerkleProofObject, error) {
	smtName := zetosmt.MerkleTreeName(tokenName, contractAddress)
	if forLockedStates {
		smtName = zetosmt.MerkleTreeNameForLockedStates(tokenName, contractAddress)
	}
	smtType := smt.StatesTree
	if forLockedStates {
		smtType = smt.LockedStatesTree
	}

	mt, err := common.NewMerkleTreeSpec(ctx, smtName, smtType, callbacks, merkleTreeRootSchema.Id, merkleTreeNodeSchema.Id, stateQueryContext)
	if err != nil {
		return nil, err
	}

	var indexes []*big.Int
	indexes, err = makeLeafIndexesFromCoins(ctx, inputCoins, mt.Tree, hasher)
	if err != nil {
		return nil, err
	}

	smtProof, err := generateMerkleProofs(ctx, mt, indexes, targetSize)
	if err != nil {
		return nil, err
	}
	return smtProof, nil
}

func smtProofForOwners(ctx context.Context, callbacks plugintk.DomainCallbacks, merkleTreeRootSchema *prototk.StateSchema, merkleTreeNodeSchema *prototk.StateSchema, tokenName, stateQueryContext string, contractAddress *pldtypes.EthAddress, inputOwner string, outputCoins []*types.ZetoCoin, targetSize int) (*corepb.MerkleProofObject, error) {
	smtName := zetosmt.MerkleTreeNameForKycStates(tokenName, contractAddress)
	mt, err := common.NewMerkleTreeSpec(ctx, smtName, smt.KycStatesTree, callbacks, merkleTreeRootSchema.Id, merkleTreeNodeSchema.Id, stateQueryContext)
	if err != nil {
		return nil, err
	}

	// for KYC, we need to collect the indexes from the coins owners
	indexes, err := makeLeafIndexesFromCoinOwners(ctx, inputOwner, outputCoins)
	if err != nil {
		return nil, err
	}

	smtProof, err := generateMerkleProofs(ctx, mt, indexes, targetSize)
	if err != nil {
		return nil, err
	}
	return smtProof, nil
}

// trimZeroUtxos drops "0" padding from utxosFromOutputStates slices before on-chain lock/createLock/spendLock calldata.
// The pool re-pads inside checkAndPadCommitments; raw array lengths must not force the batch verifier when the proof is 2×2 anon.
func trimZeroUtxos(utxos []string) []string {
	trimmed := make([]string, 0, len(utxos))
	for _, utxo := range utxos {
		if utxo != "0" {
			trimmed = append(trimmed, utxo)
		}
	}
	return trimmed
}

// lockTransitionOutputCoinsForProof orders change (unlocked) and locked mint outputs to match the on-chain vector passed
// to verifyProof after concatenating lock() / createLock args (see types.LockTransitionVerifierOutputOrderLockedFirst).
func lockTransitionOutputCoinsForProof(zetoVariant pldtypes.HexUint64, unlockedChange, locked []*types.ZetoCoin) []*types.ZetoCoin {
	if types.LockTransitionVerifierOutputOrderLockedFirst(zetoVariant) {
		return slices.Concat(locked, unlockedChange)
	}
	return slices.Concat(unlockedChange, locked)
}

func getAlgoZetoSnarkBJJ(name string) string {
	return zetosignerapi.AlgoDomainZetoSnarkBJJ(name)
}

// qualifyPartyLookup maps a short Paladin identity (e.g. "controller") to the node-qualified form used in
// ResolvedVerifiers (e.g. "controller@node1"), matching testbed_prepare and the private TX manager.
func qualifyPartyLookup(lookup, txFrom string) string {
	lookup = strings.TrimSpace(lookup)
	if lookup == "" || strings.Contains(lookup, "@") {
		return lookup
	}
	if at := strings.Index(txFrom, "@"); at > 0 {
		return lookup + txFrom[at:]
	}
	return lookup
}

// cancelRecipientsFromLockInfo parses cancelData JSON pinned at createLock.
func cancelRecipientsFromLockInfo(ctx context.Context, li *types.ZetoLockInfoState) ([]*types.FungibleTransferParamEntry, error) {
	if len(li.CancelData) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoMissingCancelData, li.LockID.HexString0xPrefix())
	}
	var parsed []*types.FungibleTransferParamEntry
	if err := json.Unmarshal(li.CancelData, &parsed); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "invalid cancelData (recipients JSON) in lock info")
	}
	if len(parsed) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoMissingCancelData, li.LockID.HexString0xPrefix())
	}
	return parsed, nil
}

// cancelRecipientsForLockInfoJSON returns the single owner return entry for cancelLock (locked collateral total).
func cancelRecipientsForLockInfoJSON(owner string, lockedTotal *pldtypes.HexUint256) ([]byte, error) {
	entry := &types.FungibleTransferParamEntry{
		To:     qualifyPartyLookup(owner, owner),
		Amount: lockedTotal,
	}
	return json.Marshal([]*types.FungibleTransferParamEntry{entry})
}

// recipientsForLockInfoJSON returns createLock recipients with Paladin To lookups aligned to tx.Transaction.From
// so spendLock assemble can resolve BJJ verifiers from Init.
func recipientsForLockInfoJSON(params *types.CreateLockParams, txFrom string) ([]byte, error) {
	if params == nil {
		return json.Marshal([]*types.FungibleTransferParamEntry{})
	}
	out := make([]*types.FungibleTransferParamEntry, 0, len(params.Recipients))
	for _, r := range params.Recipients {
		if r == nil {
			continue
		}
		entry := *r
		entry.To = qualifyPartyLookup(entry.To, txFrom)
		out = append(out, &entry)
	}
	return json.Marshal(out)
}

func marshalTokenSecrets(input, output []uint64) ([]byte, error) {
	return json.Marshal(corepb.TokenSecrets_Fungible{InputValues: input, OutputValues: output})
}
