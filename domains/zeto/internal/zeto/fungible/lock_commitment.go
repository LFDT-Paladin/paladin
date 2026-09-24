/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// zetoSpendLockEmptyLockedOutputs is the lockedOutputs vector in ZetoSpendLockArgs for spendLock/cancelLock on
// v0.5 fungible pools (locked collateral is read from lock storage; args.lockedOutputs is empty).
var zetoSpendLockEmptyLockedOutputs = []string{}

// computeCreateLockCommitments builds spend/cancel commitments matching IZetoLockableCapability.computeSpendHash /
// computeCancelHash for the pre-pinned spend and cancel paths.
func computeCreateLockCommitments(
	ctx context.Context,
	lockedInputs []string,
	proposedSpendCoins, proposedCancelCoins []*types.ZetoCoin,
	spendData, cancelData []byte,
) (spendCommitment, cancelCommitment pldtypes.Bytes32, err error) {
	inputSize := common.GetInputSize(max(len(proposedSpendCoins), len(proposedCancelCoins), len(lockedInputs)))
	spendOutputUTXOs, err := utxosFromCoins(ctx, proposedSpendCoins, inputSize)
	if err != nil {
		return pldtypes.Bytes32{}, pldtypes.Bytes32{}, err
	}
	cancelOutputUTXOs, err := utxosFromCoins(ctx, proposedCancelCoins, inputSize)
	if err != nil {
		return pldtypes.Bytes32{}, pldtypes.Bytes32{}, err
	}
	spendCommitment, err = types.ComputeZetoSpendCommitment(ctx, lockedInputs, zetoSpendLockEmptyLockedOutputs, trimZeroUtxos(spendOutputUTXOs), spendData)
	if err != nil {
		return pldtypes.Bytes32{}, pldtypes.Bytes32{}, err
	}
	cancelCommitment, err = types.ComputeZetoCancelCommitment(ctx, lockedInputs, zetoSpendLockEmptyLockedOutputs, trimZeroUtxos(cancelOutputUTXOs), cancelData)
	return spendCommitment, cancelCommitment, err
}

func lockInfoFromOutputStates(ctx context.Context, lockInfoSchema *prototk.StateSchema, outputStates []*prototk.EndorsableState) (*types.ZetoLockInfoState, error) {
	if lockInfoSchema == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "lock info schema is not configured")
	}
	for _, state := range outputStates {
		if state.GetSchemaId() != lockInfoSchema.Id {
			continue
		}
		var info types.ZetoLockInfoState
		if err := json.Unmarshal([]byte(state.StateDataJson), &info); err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
		}
		return &info, nil
	}
	return nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, "lock info state not found in createLock outputs")
}

func max(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}
