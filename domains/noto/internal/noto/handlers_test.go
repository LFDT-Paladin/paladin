// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package noto

import (
	"math/big"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/require"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

func testCoins(owner *pldtypes.EthAddress, amounts ...int64) *parsedCoins {
	pc := &parsedCoins{total: big.NewInt(0), lockedTotal: big.NewInt(0)}
	for _, a := range amounts {
		pc.coins = append(pc.coins, &types.NotoCoin{Owner: owner, Amount: pldtypes.Int64ToInt256(a)})
		pc.states = append(pc.states, &prototk.StateRef{Id: pldtypes.RandBytes32().String()})
		pc.total = new(big.Int).Add(pc.total, big.NewInt(a))
	}
	return pc
}

func testLockedCoins(owner *pldtypes.EthAddress, amounts ...int64) *parsedCoins {
	pc := &parsedCoins{total: big.NewInt(0), lockedTotal: big.NewInt(0)}
	for _, a := range amounts {
		pc.lockedCoins = append(pc.lockedCoins, &types.NotoLockedCoin{Owner: owner, Amount: pldtypes.Int64ToInt256(a)})
		pc.lockedStates = append(pc.lockedStates, &prototk.StateRef{Id: pldtypes.RandBytes32().String()})
		pc.lockedTotal = new(big.Int).Add(pc.lockedTotal, big.NewInt(a))
	}
	return pc
}

// The amount validators all compare states an originator assembled against what the
// transaction said it would do, so a mismatch invalidates the transaction outright.

func TestValidateMintAmountsRevertsOnInputs(t *testing.T) {
	owner := pldtypes.RandAddress()
	params := &types.MintParams{Amount: pldtypes.Int64ToInt256(100)}
	err := (&Noto{}).validateMintAmounts(t.Context(), params, testCoins(owner, 10), testCoins(owner, 100))
	assertRevert(t, err, "PD200012.*mint")
}

func TestValidateMintAmountsRevertsOnMismatch(t *testing.T) {
	owner := pldtypes.RandAddress()
	params := &types.MintParams{Amount: pldtypes.Int64ToInt256(100)}
	err := (&Noto{}).validateMintAmounts(t.Context(), params, testCoins(owner), testCoins(owner, 99))
	assertRevert(t, err, "PD200013.*mint")
}

func TestValidateTransferAmountsRevertsOnNoInputs(t *testing.T) {
	owner := pldtypes.RandAddress()
	err := (&Noto{}).validateTransferAmounts(t.Context(), testCoins(owner), testCoins(owner, 100))
	assertRevert(t, err, "PD200012.*transfer")
}

func TestValidateTransferAmountsRevertsOnMismatch(t *testing.T) {
	owner := pldtypes.RandAddress()
	err := (&Noto{}).validateTransferAmounts(t.Context(), testCoins(owner, 100), testCoins(owner, 99))
	assertRevert(t, err, "PD200013.*transfer")
}

func TestValidateBurnAmountsRevertsOnNoInputs(t *testing.T) {
	owner := pldtypes.RandAddress()
	params := &types.BurnParams{Amount: pldtypes.Int64ToInt256(100)}
	err := (&Noto{}).validateBurnAmounts(t.Context(), params, testCoins(owner), testCoins(owner))
	assertRevert(t, err, "PD200012.*burn")
}

func TestValidateBurnAmountsRevertsOnMismatch(t *testing.T) {
	owner := pldtypes.RandAddress()
	params := &types.BurnParams{Amount: pldtypes.Int64ToInt256(100)}
	err := (&Noto{}).validateBurnAmounts(t.Context(), params, testCoins(owner, 100), testCoins(owner, 50))
	assertRevert(t, err, "PD200013.*burn")
}

// V0 did not support empty locks, so a lock with nothing to lock was invalid
func TestValidateLockAmountsRevertsOnNoInputsInV0(t *testing.T) {
	owner := pldtypes.RandAddress()
	tx := &types.ParsedTransaction{DomainConfig: notoBasicConfigV0}
	err := (&Noto{}).validateLockAmounts(t.Context(), tx, testCoins(owner), testCoins(owner))
	assertRevert(t, err, "PD200012.*lock")
}

func TestValidateLockAmountsRevertsOnMismatch(t *testing.T) {
	owner := pldtypes.RandAddress()
	tx := &types.ParsedTransaction{DomainConfig: notoBasicConfigV1}
	err := (&Noto{}).validateLockAmounts(t.Context(), tx, testCoins(owner, 100), testCoins(owner, 50))
	assertRevert(t, err, "PD200013.*lock")
}

// In V0 there was no lock object, so the locked inputs were the only evidence of the lock
func TestValidateUnlockAmountsRevertsOnNoLockedInputsInV0(t *testing.T) {
	owner := pldtypes.RandAddress()
	tx := &types.ParsedTransaction{DomainConfig: notoBasicConfigV0}
	err := (&Noto{}).validateUnlockAmounts(t.Context(), tx, testCoins(owner), testCoins(owner))
	assertRevert(t, err, "PD200012.*unlock")
}

func TestValidateUnlockAmountsRevertsOnMismatch(t *testing.T) {
	owner := pldtypes.RandAddress()
	tx := &types.ParsedTransaction{DomainConfig: notoBasicConfigV1}
	err := (&Noto{}).validateUnlockAmounts(t.Context(), tx, testLockedCoins(owner, 100), testCoins(owner, 50))
	assertRevert(t, err, "PD200013.*unlock")
}

func TestValidateOwnersRevertsOnWrongOwner(t *testing.T) {
	n := &Noto{}
	owner := pldtypes.RandAddress()
	someoneElse := pldtypes.RandAddress()
	resolved := []*prototk.ResolvedVerifier{{
		Lookup:       "sender@node1",
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		Verifier:     someoneElse.String(),
	}}

	inputs := testCoins(owner, 100)
	assertRevert(t, n.validateOwners(t.Context(), "sender@node1", resolved, inputs.coins, inputs.states), "PD200018")

	locked := testLockedCoins(owner, 100)
	assertRevert(t, n.validateLockOwners(t.Context(), "sender@node1", resolved, locked.lockedCoins, locked.lockedStates), "PD200018")
}

func TestFindEthAddressVerifierEndorseOnly(t *testing.T) {
	n := &Noto{}

	_, err := n.findEthAddressVerifier(t.Context(), "sender", "sender@node1", nil)
	assertEndorseOnlyRevert(t, err, "PD200011.*'sender'")

	_, err = n.findEthAddressVerifier(t.Context(), "sender", "sender@node1", []*prototk.ResolvedVerifier{{
		Lookup:       "sender@node1",
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		Verifier:     "not an address",
	}})
	assertEndorseOnlyRevert(t, err, "bad address")
}

// signedBy returns the attestation an originator would supply for a message, and the message
// it signed, so each test below can break exactly one thing about it.
func signedBy(t *testing.T, verifier string) ([]byte, []*prototk.AttestationResult) {
	message := []byte("the encoded transfer")
	senderKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)
	signature, err := senderKey.SignDirect(message)
	require.NoError(t, err)
	if verifier == "" {
		verifier = senderKey.Address.String()
	}
	return message, []*prototk.AttestationResult{{
		Name:     "sender",
		Payload:  signature.CompactRSV(),
		Verifier: &prototk.ResolvedVerifier{Verifier: verifier},
	}}
}

// The signature is supplied by the originator alongside the transaction, so every way it can
// fail to establish that the originator authorised these inputs is a revert.

func TestValidateSignatureRevertsWhenAbsent(t *testing.T) {
	assertRevert(t, (&Noto{}).validateSignature(t.Context(), "sender", nil, []byte("the encoded transfer")), "PD200015.*'sender'")
}

func TestValidateSignatureRevertsWhenItWillNotRecover(t *testing.T) {
	message, attestations := signedBy(t, "")
	attestations[0].Payload = []byte("not a signature")
	assertRevert(t, (&Noto{}).validateSignature(t.Context(), "sender", attestations, message), "FF22087")
}

func TestValidateSignatureRevertsWhenItRecoversToSomeoneElse(t *testing.T) {
	message, attestations := signedBy(t, pldtypes.RandAddress().String())
	assertRevert(t, (&Noto{}).validateSignature(t.Context(), "sender", attestations, message), "PD200017.*'sender'")
}

// notoForLockEndorse builds a Noto with the schemas a V1 lock endorsement touches, and a
// hooks-mode transaction so the notary-mode checks pass and the amount checks are reached.
func notoForLockEndorse() (*Noto, *types.ParsedTransaction, []*prototk.ResolvedVerifier, *pldtypes.EthAddress) {
	n := &Noto{
		coinSchema:       testSchema("coin"),
		lockedCoinSchema: testSchema("lockedCoin"),
		lockInfoSchemaV1: testSchema("lockInfoV1"),
	}
	sender := pldtypes.RandAddress()
	tx := &types.ParsedTransaction{
		Transaction:     &prototk.TransactionSpecification{From: "sender@node1"},
		ContractAddress: ethtypes.MustNewAddress(pldtypes.RandAddress().String()),
		DomainConfig: &types.NotoParsedConfig{
			NotaryMode:   types.NotaryModeHooks.Enum(),
			NotaryLookup: "notary@node1",
			Variant:      types.NotoVariantV2,
		},
	}
	resolved := []*prototk.ResolvedVerifier{{
		Lookup:       "sender@node1",
		Algorithm:    algorithms.ECDSA_SECP256K1,
		VerifierType: verifiers.ETH_ADDRESS,
		Verifier:     sender.String(),
	}}
	return n, tx, resolved, sender
}

// newLockState builds the single output lock-info state a LOCK_CREATE transition produces.
func newLockState(owner *pldtypes.EthAddress, spendOutputs, cancelOutputs []pldtypes.Bytes32) *prototk.EndorsableState {
	info := &types.NotoLockInfo_V1{
		Salt:          pldtypes.RandBytes32(),
		LockID:        pldtypes.RandBytes32(),
		Owner:         owner,
		Spender:       owner,
		SpendOutputs:  spendOutputs,
		CancelOutputs: cancelOutputs,
	}
	if len(spendOutputs) > 0 {
		info.SpendTxId = pldtypes.RandBytes32()
	}
	return &prototk.EndorsableState{
		SchemaId:      hashName("lockInfoV1"),
		Id:            pldtypes.RandBytes32().String(),
		StateDataJson: mustParseJSON(info),
	}
}

// newCoinState builds an unlocked coin state, and newLockedCoinState its locked counterpart.
func newCoinState(owner *pldtypes.EthAddress, amount int64) *prototk.EndorsableState {
	return &prototk.EndorsableState{
		SchemaId: hashName("coin"),
		Id:       pldtypes.RandBytes32().String(),
		StateDataJson: mustParseJSON(&types.NotoCoin{
			Salt: pldtypes.RandBytes32(), Owner: owner, Amount: pldtypes.Int64ToInt256(amount),
		}),
	}
}

func newLockedCoinState(owner *pldtypes.EthAddress, amount int64) *prototk.EndorsableState {
	return &prototk.EndorsableState{
		SchemaId: hashName("lockedCoin"),
		Id:       pldtypes.RandBytes32().String(),
		StateDataJson: mustParseJSON(&types.NotoLockedCoin{
			Salt: pldtypes.RandBytes32(), LockID: pldtypes.RandBytes32(), Owner: owner, Amount: pldtypes.Int64ToInt256(amount),
		}),
	}
}

func stateIDs(states ...*prototk.EndorsableState) []pldtypes.Bytes32 {
	ids := make([]pldtypes.Bytes32, len(states))
	for i, s := range states {
		ids[i] = pldtypes.MustParseBytes32(s.Id)
	}
	return ids
}
