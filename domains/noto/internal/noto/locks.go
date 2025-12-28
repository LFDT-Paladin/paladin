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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type loadedLockInfo struct {
	id       pldtypes.Bytes32
	stateRef *prototk.StateRef
	lockInfo *types.NotoLockInfo_V1
}

type lockTransitionType int

const (
	LOCK_DECODE_ANY lockTransitionType = iota
	LOCK_CREATE
	LOCK_UPDATE
	LOCK_SPEND
)

type lockTransition struct {
	noto          *Noto
	prevLockState *prototk.EndorsableState
	prevLockInfo  types.NotoLockInfo_V1
	newLockState  *prototk.EndorsableState
	newLockInfo   types.NotoLockInfo_V1
}

func (n *Noto) loadLockInfoV1(ctx context.Context, stateQueryContext string, lockID pldtypes.Bytes32) (*loadedLockInfo, error) {
	queryBuilder := query.NewQueryBuilder().
		Limit(1).
		Sort("-.created").
		Equal("lockId", lockID)
	log.L(ctx).Debugf("Lock query: %s", queryBuilder.Query())
	states, err := n.findAvailableStates(ctx, stateQueryContext, n.lockInfoSchemaV1.Id, queryBuilder.Query().String())
	if err != nil {
		return nil, err
	}
	if len(states) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgLockIDNotFound)
	}
	var lockState = states[0]
	var lockInfo types.NotoLockInfo_V1
	lockStateID, err := pldtypes.ParseBytes32(lockState.Id)
	if err == nil {
		err = json.Unmarshal([]byte(lockState.DataJson), &lockInfo)
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidLockState, lockState.Id)
	}
	return &loadedLockInfo{
		id:       lockStateID,
		stateRef: &prototk.StateRef{Id: lockState.Id, SchemaId: lockState.SchemaId},
		lockInfo: &lockInfo,
	}, nil
}

// takes an assembled V1 lock transition (including a new lock), does basic validation & parsing,
// then returns the parsed result for further checking/processing.
func (n *Noto) validateV1LockTransition(ctx context.Context, transitionType lockTransitionType, fromID *identityPair, lockID *pldtypes.Bytes32, inputs []*prototk.EndorsableState, outputs []*prototk.EndorsableState) (*lockTransition, error) {
	var lt lockTransition
	lt.noto = n

	inputLockInfoStates := n.filterSchema(inputs, []string{n.lockInfoSchemaV1.Id})
	outputLockInfoStates := n.filterSchema(outputs, []string{n.lockInfoSchemaV1.Id})

	switch transitionType {
	case LOCK_CREATE:
		if len(inputLockInfoStates) != 0 || len(outputLockInfoStates) != 1 {
			return nil, i18n.NewError(ctx, msgs.MsgInvalidLockTransition)
		}
	case LOCK_SPEND:
		if len(inputLockInfoStates) != 1 || len(outputLockInfoStates) != 0 {
			return nil, i18n.NewError(ctx, msgs.MsgInvalidLockTransition)
		}
	case LOCK_UPDATE:
		if len(inputLockInfoStates) != 1 || len(outputLockInfoStates) != 1 {
			return nil, i18n.NewError(ctx, msgs.MsgInvalidLockTransition)
		}
	}

	if len(inputLockInfoStates) == 1 {
		lt.prevLockState = inputLockInfoStates[0]
		if err := json.Unmarshal([]byte(lt.prevLockState.StateDataJson), &lt.prevLockInfo); err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidLockState, lt.prevLockState.Id)
		}

		// Check ownership of the input lock is the from address of the transaction
		if fromID != nil && !lt.prevLockInfo.Owner.Equals(fromID.address) {
			return nil, i18n.NewError(ctx, msgs.MsgStateWrongOwner, lt.prevLockState.Id, fromID.address)
		}
	}

	if len(outputLockInfoStates) == 1 {
		lt.newLockState = outputLockInfoStates[0]
		if err := json.Unmarshal([]byte(lt.newLockState.StateDataJson), &lt.newLockInfo); err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidLockState, lt.newLockState.Id)
		}

		// New lock state must be a valid lock state
		if lt.newLockInfo.Salt.IsZero() || lt.newLockInfo.Owner == nil || lt.newLockInfo.Spender == nil ||
			(len(lt.newLockInfo.SpendOutputs) > 0 && lt.newLockInfo.SpendTxId.IsZero()) {
			return nil, i18n.NewError(ctx, msgs.MsgInvalidLockState, lt.newLockState.Id)
		}
		// VAlidate the lockId if we were passed one
		if lockID != nil && !lt.newLockInfo.LockID.Equals(lockID) {
			return nil, i18n.NewError(ctx, msgs.MsgInvalidLockStateLockID, lt.newLockState.Id, lockID, lt.newLockInfo.LockID)
		}

		// If we have an old lock state, the transition must be valid
		if lt.prevLockState != nil {
			if !lt.newLockInfo.Owner.Equals(lt.prevLockInfo.Owner) || // owner must not change
				!lt.newLockInfo.LockID.Equals(&lt.prevLockInfo.LockID) || // lock ID must not change
				lt.newLockInfo.Replaces.String() != lt.prevLockState.Id || // back pointer must be correct
				lt.newLockInfo.Salt.Equals(&lt.prevLockInfo.Salt) { // and the salt must change
				log.L(ctx).Errorf("Invalid lock transition. old=%s new=%s", lt.prevLockState.StateDataJson, lt.newLockState.StateDataJson)
				return nil, i18n.NewError(ctx, msgs.MsgInvalidLockTransition)
			}
		}
	}

	return &lt, nil
}

func (lt *lockTransition) splitOutputs(ctx context.Context, infoStates []*prototk.EndorsableState) (spendOutputs, cancelOutputs []*prototk.EndorsableState, err error) {
	coinOutputsDebug := make([]string, 0, len(infoStates))
	for _, output := range lt.noto.filterSchema(infoStates, []string{lt.noto.coinSchema.Id}) {
		coinOutputsDebug = append(coinOutputsDebug, output.StateDataJson)
		for _, expectedSpendOutput := range lt.newLockInfo.SpendOutputs {
			if output.Id == expectedSpendOutput.String() {
				spendOutputs = append(spendOutputs, output)
			}
		}
		for _, expectedCancelOutput := range lt.newLockInfo.CancelOutputs {
			if output.Id == expectedCancelOutput.String() {
				cancelOutputs = append(cancelOutputs, output)
			}
		}
	}
	if len(spendOutputs) != len(lt.newLockInfo.SpendOutputs) || len(cancelOutputs) != len(lt.newLockInfo.CancelOutputs) {
		log.L(ctx).Errorf("Invalid info states for transition. coins=%v lock=%s", coinOutputsDebug, lt.newLockState.StateDataJson)
		return nil, nil, i18n.NewError(ctx, msgs.MsgInvalidLockTransition)
	}
	return spendOutputs, cancelOutputs, nil
}

func (n *Noto) decodeV1LockTransitionWithOutputs(ctx context.Context, transitionType lockTransitionType, senderID *identityPair, lockID *pldtypes.Bytes32, inputs []*prototk.EndorsableState, outputs []*prototk.EndorsableState, infoStates []*prototk.EndorsableState) (lt *lockTransition, spendOutputs, cancelOutputs []*prototk.EndorsableState, err error) {
	lt, err = n.validateV1LockTransition(ctx, transitionType, senderID, lockID, inputs, outputs)
	if err == nil {
		spendOutputs, cancelOutputs, err = lt.splitOutputs(ctx, infoStates)
	}
	return
}
