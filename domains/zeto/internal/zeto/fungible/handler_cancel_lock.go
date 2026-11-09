/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	pb "github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

var _ types.DomainHandler = &cancelLockHandler{}

type cancelLockHandler struct {
	unlockCommon
}

func NewCancelLockHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema, lockInfoSchema *pb.StateSchema) *cancelLockHandler {
	return &cancelLockHandler{
		unlockCommon: newUnlockCommon(name, callbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema, lockInfoSchema),
	}
}

func (h *cancelLockHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var p types.CancelLockParams
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return nil, err
	}
	if err := validateLockUnlockParams(ctx, p.LockId, p.From); err != nil {
		return nil, err
	}
	return &p, nil
}

func (h *cancelLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	return h.init(tx), nil
}

func (h *cancelLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	return h.assemble(ctx, tx, req, cancelLockOperation, tx.Params.(*types.CancelLockParams).LockId)
}

func (h *cancelLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *cancelLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	return h.prepare(ctx, tx, req, cancelLockOperation, tx.Params.(*types.CancelLockParams).LockId)
}
