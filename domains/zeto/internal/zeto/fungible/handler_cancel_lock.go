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
