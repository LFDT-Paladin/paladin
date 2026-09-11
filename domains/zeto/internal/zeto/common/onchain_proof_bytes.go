/*
 * Copyright © 2025 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"context"
	"encoding/json"
	"strings"

	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

// EncodeZetoOnchainTransferProofBytes ABI-encodes the `proof` bytes passed to ZetoFungible.transfer when
// UseZetoOnchainPackedProofCalldata is true. Layout follows constructPublicInputs in upstream contracts
// (e.g. zeto_anon.sol, zeto_anon_nullifier.sol, zeto_anon_enc.sol).
func EncodeZetoOnchainTransferProofBytes(ctx context.Context, tokenName string, proof *corepb.SnarkProof, publicInputs map[string]string, lockedInputs bool) ([]byte, error) {
	switch {
	case IsEncryptionToken(tokenName):
		return encodeEncryptionProofPayload(ctx, proof, publicInputs)
	case IsNullifiersToken(tokenName):
		if lockedInputs {
			return encodeProofTupleOnly(ctx, proof)
		}
		return encodeRootAndProofTuple(ctx, proof, publicInputs["root"])
	default:
		return encodeProofTupleOnly(ctx, proof)
	}
}

// EncodeZetoOnchainWithdrawProofBytes ABI-encodes withdraw's `proof` bytes (ZetoFungibleNullifier uses root + tuple).
func EncodeZetoOnchainWithdrawProofBytes(ctx context.Context, tokenName string, proof *corepb.SnarkProof, publicInputs map[string]string) ([]byte, error) {
	if IsNullifiersToken(tokenName) {
		return encodeRootAndProofTuple(ctx, proof, publicInputs["root"])
	}
	return encodeProofTupleOnly(ctx, proof)
}

// EncodeZetoOnchainDepositProofBytes ABI-encodes deposit's `proof` bytes (always Groth16 tuple only on chain).
func EncodeZetoOnchainDepositProofBytes(ctx context.Context, proof *corepb.SnarkProof) ([]byte, error) {
	return encodeProofTupleOnly(ctx, proof)
}

func encodeProofTupleOnly(ctx context.Context, proof *corepb.SnarkProof) ([]byte, error) {
	proofJSON, err := json.Marshal(EncodeProof(proof))
	if err != nil {
		return nil, err
	}
	return ProofComponents.EncodeABIDataJSONCtx(ctx, proofJSON)
}

func encodeRootAndProofTuple(ctx context.Context, proof *corepb.SnarkProof, root string) ([]byte, error) {
	payload := map[string]any{
		"root":  strings.TrimSpace(root),
		"proof": EncodeProof(proof),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	pa := abi.ParameterArray{
		{Name: "root", Type: "uint256"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: ProofComponents},
	}
	return pa.EncodeABIDataJSONCtx(ctx, payloadJSON)
}

func encodeEncryptionProofPayload(ctx context.Context, proof *corepb.SnarkProof, publicInputs map[string]string) ([]byte, error) {
	ecdhRaw := strings.TrimSpace(publicInputs["ecdhPublicKey"])
	var ecdhParts []string
	if ecdhRaw == "" {
		ecdhParts = []string{"0x0", "0x0"}
	} else {
		ecdhParts = strings.Split(ecdhRaw, ",")
		for len(ecdhParts) < 2 {
			ecdhParts = append(ecdhParts, "0x0")
		}
		if len(ecdhParts) > 2 {
			ecdhParts = ecdhParts[:2]
		}
		for i := range ecdhParts {
			if strings.TrimSpace(ecdhParts[i]) == "" {
				ecdhParts[i] = "0x0"
			} else {
				ecdhParts[i] = strings.TrimSpace(ecdhParts[i])
			}
		}
	}
	encVals := strings.Split(publicInputs["encryptedValues"], ",")
	payload := map[string]any{
		"encryptionNonce": strings.TrimSpace(publicInputs["encryptionNonce"]),
		"ecdhPublicKey": []string{ecdhParts[0], ecdhParts[1]},
		"encryptedValues": func() []string {
			out := make([]string, 0, len(encVals))
			for _, v := range encVals {
				out = append(out, strings.TrimSpace(v))
			}
			return out
		}(),
		"groth16Proof": EncodeProof(proof),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	pa := abi.ParameterArray{
		{Name: "encryptionNonce", Type: "uint256"},
		{Name: "ecdhPublicKey", Type: "uint256[2]"},
		{Name: "encryptedValues", Type: "uint256[]"},
		{Name: "groth16Proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: ProofComponents},
	}
	return pa.EncodeABIDataJSONCtx(ctx, payloadJSON)
}
