/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package zeto

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNFTSchemaID(t *testing.T) {
	z := &Zeto{nftSchema: &prototk.StateSchema{Id: "nft"}}
	assert.Equal(t, "nft", z.NFTSchemaID())
}

func TestCheckStateCompletion(t *testing.T) {
	z := &Zeto{}
	missing := "state-missing"
	res, err := z.CheckStateCompletion(context.Background(), &prototk.CheckStateCompletionRequest{
		UnavailableStates: &prototk.UnavailableStates{FirstUnavailableId: &missing},
	})
	require.NoError(t, err)
	require.NotNil(t, res.NextMissingStateId)
	assert.Equal(t, "state-missing", *res.NextMissingStateId)
}

func balanceOfCallTxSpec(t *testing.T) *prototk.TransactionSpecification {
	t.Helper()
	fnABI := types.ZetoFungibleFunctionForVariant(types.ZetoFungibleV0ABI, types.METHOD_BALANCE_OF)
	require.NotNil(t, fnABI)
	fnJSON, err := json.Marshal(fnABI)
	require.NoError(t, err)
	domainConfig := &types.DomainInstanceConfig{TokenName: constants.TOKEN_ANON, ZetoVariant: types.ZetoFungibleV0ABI}
	dcJSON, err := json.Marshal(domainConfig)
	require.NoError(t, err)
	paramsJSON, err := json.Marshal(&types.FungibleBalanceOfParam{Account: "alice@node"})
	require.NoError(t, err)
	return &prototk.TransactionSpecification{
		TransactionId:      "0x" + strings.Repeat("aa", 32),
		From:                 "bob@node",
		FunctionAbiJson:      string(fnJSON),
		FunctionSignature:    fnABI.SolString(),
		FunctionParamsJson:   string(paramsJSON),
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    "0x1234567890123456789012345678901234567890",
			ContractConfigJson: string(dcJSON),
		},
	}
}

func TestInitCall_BalanceOf(t *testing.T) {
	ctx := context.Background()
	z, _ := newTestZeto()
	txSpec := balanceOfCallTxSpec(t)
	res, err := z.InitCall(ctx, &prototk.InitCallRequest{Transaction: txSpec})
	require.NoError(t, err)
	require.Len(t, res.RequiredVerifiers, 1)
	assert.Equal(t, "alice@node", res.RequiredVerifiers[0].Lookup)
}

func TestExecCall_BalanceOf(t *testing.T) {
	ctx := context.Background()
	z, cb := newTestZeto()
	txSpec := balanceOfCallTxSpec(t)
	cb.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{{
				DataJson: `{"salt":"0x01","owner":"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922","amount":"0x05"}`,
			}},
		}, nil
	}
	initRes, err := z.InitCall(ctx, &prototk.InitCallRequest{Transaction: txSpec})
	require.NoError(t, err)
	execRes, err := z.ExecCall(ctx, &prototk.ExecCallRequest{
		Transaction: txSpec,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{{
			Lookup:       initRes.RequiredVerifiers[0].Lookup,
			Verifier:     "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922",
			Algorithm:    initRes.RequiredVerifiers[0].Algorithm,
			VerifierType: initRes.RequiredVerifiers[0].VerifierType,
		}},
		StateQueryContext: "ctx1",
	})
	require.NoError(t, err)
	assert.Contains(t, execRes.ResultJson, "totalBalance")
}

func TestValidateCall_InvalidABI(t *testing.T) {
	ctx := context.Background()
	z, _ := newTestZeto()
	_, _, err := z.validateCall(ctx, &prototk.TransactionSpecification{FunctionAbiJson: "not json"})
	require.Error(t, err)
}

func TestInitCall_InvalidTx(t *testing.T) {
	ctx := context.Background()
	z, _ := newTestZeto()
	_, err := z.InitCall(ctx, &prototk.InitCallRequest{Transaction: &prototk.TransactionSpecification{FunctionAbiJson: "bad"}})
	require.Error(t, err)
}

func TestExecCall_InvalidCallSpec(t *testing.T) {
	ctx := context.Background()
	z, _ := newTestZeto()
	_, err := z.ExecCall(ctx, &prototk.ExecCallRequest{
		Transaction: &prototk.TransactionSpecification{FunctionAbiJson: "not valid abi"},
	})
	require.Error(t, err)
}

func TestExecCall_CallbackError(t *testing.T) {
	ctx := context.Background()
	z, cb := newTestZeto()
	txSpec := balanceOfCallTxSpec(t)
	cb.MockFindAvailableStates = func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		return nil, errors.New("query failed")
	}
	initRes, err := z.InitCall(ctx, &prototk.InitCallRequest{Transaction: txSpec})
	require.NoError(t, err)
	_, err = z.ExecCall(ctx, &prototk.ExecCallRequest{
		Transaction:       txSpec,
		StateQueryContext: "ctx1",
		ResolvedVerifiers: []*prototk.ResolvedVerifier{{
			Lookup:       initRes.RequiredVerifiers[0].Lookup,
			Verifier:     "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922",
			Algorithm:    initRes.RequiredVerifiers[0].Algorithm,
			VerifierType: initRes.RequiredVerifiers[0].VerifierType,
		}},
	})
	require.Error(t, err)
}
