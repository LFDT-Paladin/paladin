/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	pb "github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

// zetoLockUnlockArgsTupleABI encodes IZetoLockableCapability.ZetoSpendLockArgs for spendLock and cancelLock.
var zetoLockUnlockArgsTupleABI = abi.ParameterArray{
	{
		Type:         "tuple",
		InternalType: "struct IZetoLockableCapability.ZetoSpendLockArgs",
		Components: abi.ParameterArray{
			{Name: "txId", Type: "bytes32"},
			{Name: "lockedOutputs", Type: "uint256[]"},
			{Name: "outputs", Type: "uint256[]"},
			{Name: "proof", Type: "bytes"},
			{Name: "data", Type: "bytes"},
		},
	},
}

// zetoLockUnlockArgsWireJSON is the inner struct for spendArgs / cancelArgs before ABI encoding.
type zetoLockUnlockArgsWireJSON struct {
	TxID          string   `json:"txId"`
	LockedOutputs []string `json:"lockedOutputs"`
	Outputs       []string `json:"outputs"`
	Proof         string   `json:"proof"`
	Data          string   `json:"data"`
}

type pinnedOutputLoad struct {
	coins  []*types.ZetoCoin
	stored []*prototk.StoredState
	total  *big.Int
}

func loadLockInfoByLockID(ctx context.Context, callbacks plugintk.DomainCallbacks, lockInfoSchema *prototk.StateSchema, lockID pldtypes.Bytes32, stateQueryContext string) (*types.ZetoLockInfoState, error) {
	if lockInfoSchema == nil {
		return nil, nil
	}
	queryJSON := query.NewQueryBuilder().Limit(1).Equal("lockId", lockID).Query().String()
	states, err := findAvailableStates(ctx, callbacks, lockInfoSchema, false, stateQueryContext, queryJSON)
	if err != nil {
		return nil, err
	}
	if len(states) == 0 {
		return nil, nil
	}
	var info types.ZetoLockInfoState
	if err := json.Unmarshal([]byte(states[0].DataJson), &info); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
	}
	return &info, nil
}

func loadLockedCoins(ctx context.Context, callbacks plugintk.DomainCallbacks, coinSchema *prototk.StateSchema, lockedOutputStrs []string, useNullifiers bool, stateQueryContext string) (*preparedInputs, bool, error) {
	lockedInputIDs := make([]string, 0, len(lockedOutputStrs))
	for _, s := range lockedOutputStrs {
		id, err := common.CoinStateIDFromPersistedString(ctx, s)
		if err != nil {
			return nil, false, err
		}
		lockedInputIDs = append(lockedInputIDs, id)
	}
	inputs, _, revert, err := loadCoinStatesByIDs(ctx, callbacks, coinSchema, useNullifiers, stateQueryContext, lockedInputIDs, true)
	return inputs, revert, err
}

func loadPinnedOutputs(ctx context.Context, callbacks plugintk.DomainCallbacks, coinSchema *prototk.StateSchema, outputIDs []string, useNullifiers bool, stateQueryContext string) (*pinnedOutputLoad, bool, error) {
	prepared, stored, revert, err := loadCoinStatesByIDs(ctx, callbacks, coinSchema, useNullifiers, stateQueryContext, outputIDs, false)
	if err != nil {
		return nil, revert, err
	}
	return &pinnedOutputLoad{coins: prepared.coins, stored: stored, total: prepared.total}, false, nil
}

func pinnedOutputStatesForNullifiers(ctx context.Context, coinSchema *prototk.StateSchema, domainName string, stateIDs []string, coins []*types.ZetoCoin, recipients []*types.FungibleTransferParamEntry) ([]*pb.NewState, error) {
	if len(stateIDs) != len(coins) || len(coins) != len(recipients) {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "pinned outputs/recipients length mismatch")
	}
	out := make([]*pb.NewState, 0, len(coins))
	for i, coin := range coins {
		ns, err := makeNewState(ctx, coinSchema, true, coin, domainName, recipients[i].To)
		if err != nil {
			return nil, err
		}
		id := stateIDs[i]
		ns.Id = &id
		out = append(out, ns)
	}
	return out, nil
}

func validateLockUnlockParams(ctx context.Context, lockID pldtypes.Bytes32, from string) error {
	if strings.TrimSpace(from) == "" {
		return i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "parameter 'from' is required")
	}
	if lockID.IsZero() {
		return i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "parameter 'lockId' is required")
	}
	return nil
}

func lockUnlockProofOutputs(ctx context.Context, h *lockUnlockHandler, lockID pldtypes.Bytes32, outputStateIDs []string, req *pb.PrepareTransactionRequest, useNullifiers bool, inputSize int) ([]string, error) {
	if len(req.OutputStates) > 0 {
		var unlockedOutputStates []*pb.EndorsableState
		coinSchemaID := h.stateSchemas.CoinSchema.Id
		for _, state := range req.OutputStates {
			if sid := state.GetSchemaId(); sid != "" && sid != coinSchemaID {
				continue
			}
			var coin types.ZetoCoin
			if err := json.Unmarshal([]byte(state.StateDataJson), &coin); err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
			}
			if !coin.Locked {
				unlockedOutputStates = append(unlockedOutputStates, state)
			}
		}
		outputs, err := utxosFromOutputStates(ctx, unlockedOutputStates, inputSize)
		if err != nil {
			return nil, err
		}
		return trimZeroUtxos(outputs), nil
	}
	coins, err := h.pinnedCoinsForPrepare(ctx, lockID, outputStateIDs, req.StateQueryContext, useNullifiers)
	if err != nil {
		return nil, err
	}
	outputs, err := utxosFromCoins(ctx, coins, inputSize)
	if err != nil {
		return nil, err
	}
	return trimZeroUtxos(outputs), nil
}

type lockUnlockHandler struct {
	baseHandler
	callbacks plugintk.DomainCallbacks
}

func (h *lockUnlockHandler) pinnedCoinsForPrepare(ctx context.Context, lockID pldtypes.Bytes32, outputStateIDs []string, stateQueryContext string, useNullifiers bool) ([]*types.ZetoCoin, error) {
	prepared, revert, err := h.loadPinnedOutputsByIDs(ctx, outputStateIDs, useNullifiers, stateQueryContext)
	if err != nil {
		return nil, err
	}
	if revert {
		return nil, i18n.NewError(ctx, msgs.MsgErrorLockInfoEmptySpendOutputs, lockID.HexString0xPrefix())
	}
	return prepared.coins, nil
}

func (h *lockUnlockHandler) loadPinnedOutputsByIDs(ctx context.Context, outputStateIDs []string, useNullifiers bool, stateQueryContext string) (*pinnedOutputLoad, bool, error) {
	return loadPinnedOutputs(ctx, h.callbacks, h.stateSchemas.CoinSchema, outputStateIDs, useNullifiers, stateQueryContext)
}
