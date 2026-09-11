/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"testing"

	signercommon "github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/signer/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmtProofForInputs_UnlockedAndLockedTrees(t *testing.T) {
	ctx := context.Background()
	contractAddress, err := pldtypes.ParseEthAddress("0x1234567890123456789012345678901234567890")
	require.NoError(t, err)
	rootSchema := &prototk.StateSchema{Id: "merkle_tree_root"}
	nodeSchema := &prototk.StateSchema{Id: "merkle_tree_node"}
	coin, err := makeCoin(merkleUTXOCoinJSON)
	require.NoError(t, err)
	cb := newMerkleNullifierCallbacks("coin", merkleUTXOCoinOwnerBJJ)

	unlocked, err := smtProofForInputs(ctx, cb, rootSchema, nodeSchema, signercommon.GetHasher(),
		"Zeto_AnonNullifier", "testContext", contractAddress, []*types.ZetoCoin{coin}, false, 2)
	require.NoError(t, err)
	assert.NotNil(t, unlocked)

	locked, err := smtProofForInputs(ctx, cb, rootSchema, nodeSchema, signercommon.GetHasher(),
		"Zeto_AnonNullifier", "testContext", contractAddress, []*types.ZetoCoin{coin}, true, 2)
	require.NoError(t, err)
	assert.NotNil(t, locked)
}

func TestSmtProofForOwners_Success(t *testing.T) {
	ctx := context.Background()
	contractAddress, err := pldtypes.ParseEthAddress("0x1234567890123456789012345678901234567890")
	require.NoError(t, err)
	rootSchema := &prototk.StateSchema{Id: "merkle_tree_root"}
	nodeSchema := &prototk.StateSchema{Id: "merkle_tree_node"}
	inputCoin, err := makeCoin(merkleUTXOCoinJSON)
	require.NoError(t, err)
	outputCoin, err := makeCoin(`{"salt":"0x142fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec","owner":"0x8cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025","amount":"0x0f","locked":false}`)
	require.NoError(t, err)
	cb := newMerkleNullifierKycCallbacks("coin")

	proof, err := smtProofForOwners(ctx, cb, rootSchema, nodeSchema, "Zeto_AnonNullifierKyc", "testContext",
		contractAddress, inputCoin.Owner.String(), []*types.ZetoCoin{outputCoin}, 2)
	require.NoError(t, err)
	assert.NotNil(t, proof)
}

func TestSmtProofForOwners_InvalidOwners(t *testing.T) {
	ctx := context.Background()
	contractAddress, err := pldtypes.ParseEthAddress("0x1234567890123456789012345678901234567890")
	require.NoError(t, err)
	rootSchema := &prototk.StateSchema{Id: "merkle_tree_root"}
	nodeSchema := &prototk.StateSchema{Id: "merkle_tree_node"}
	outputCoin, err := makeCoin(`{"salt":"0x142fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec","owner":"0x8cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025","amount":"0x0f","locked":false}`)
	require.NoError(t, err)
	cb := newMerkleNullifierKycCallbacks("coin")

	_, err = smtProofForOwners(ctx, cb, rootSchema, nodeSchema, "Zeto_AnonNullifierKyc", "testContext",
		contractAddress, "not-a-bjj-key", []*types.ZetoCoin{outputCoin}, 2)
	require.Error(t, err)

	badOwnerCoin, err := makeCoin(`{"salt":"0x142fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec","owner":"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","amount":"0x0f","locked":false}`)
	require.NoError(t, err)
	_, err = smtProofForOwners(ctx, cb, rootSchema, nodeSchema, "Zeto_AnonNullifierKyc", "testContext",
		contractAddress, merkleUTXOCoinOwnerBJJ, []*types.ZetoCoin{badOwnerCoin}, 2)
	require.Error(t, err)
}

func TestFormatTransferProvingRequest_KycNullifierWithDelegate(t *testing.T) {
	ctx := context.Background()
	contractAddress, err := pldtypes.ParseEthAddress("0x1234567890123456789012345678901234567890")
	require.NoError(t, err)
	coin, err := makeCoin(merkleUTXOCoinJSON)
	require.NoError(t, err)
	outputCoin, err := makeCoin(`{"salt":"0x142fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec","owner":"0x8cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025","amount":"0x0f","locked":false}`)
	require.NoError(t, err)
	cb := newMerkleNullifierKycCallbacks("coin")
	circuit := &zetosignerapi.Circuit{UsesNullifiers: true, UsesKyc: true}

	result, err := formatTransferProvingRequest(ctx, cb,
		&prototk.StateSchema{Id: "merkle_tree_root"},
		&prototk.StateSchema{Id: "merkle_tree_node"},
		signercommon.GetHasher(),
		[]*types.ZetoCoin{coin}, []*types.ZetoCoin{outputCoin},
		circuit, "Zeto_AnonNullifierKyc", "testContext", contractAddress, false,
		"0x74e71b05854ee819cb9397be01c82570a178d019",
	)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestFormatTransferProvingRequest_LockedStatesTreeFlag(t *testing.T) {
	ctx := context.Background()
	contractAddress, err := pldtypes.ParseEthAddress("0x1234567890123456789012345678901234567890")
	require.NoError(t, err)
	coin, err := makeCoin(merkleUTXOCoinJSON)
	require.NoError(t, err)
	cb := newMerkleNullifierCallbacks("coin", merkleUTXOCoinOwnerBJJ)
	circuit := &zetosignerapi.Circuit{UsesNullifiers: true}

	result, err := formatTransferProvingRequest(ctx, cb,
		&prototk.StateSchema{Id: "merkle_tree_root"},
		&prototk.StateSchema{Id: "merkle_tree_node"},
		signercommon.GetHasher(),
		[]*types.ZetoCoin{coin}, []*types.ZetoCoin{coin},
		circuit, "Zeto_AnonNullifier", "testContext", contractAddress, true,
		"0x74e71b05854ee819cb9397be01c82570a178d019",
	)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}
