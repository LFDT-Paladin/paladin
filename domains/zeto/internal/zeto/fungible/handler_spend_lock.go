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

var _ types.DomainHandler = &spendLockHandler{}

type spendLockHandler struct {
	unlockCommon
}

func NewSpendLockHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema, lockInfoSchema *pb.StateSchema) *spendLockHandler {
	return &spendLockHandler{
		unlockCommon: newUnlockCommon(name, callbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema, lockInfoSchema),
	}
}

func (h *spendLockHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var p types.SpendLockParams
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return nil, err
	}
	if err := validateLockUnlockParams(ctx, p.LockId, p.From); err != nil {
		return nil, err
	}
	return &p, nil
}

func (h *spendLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	return h.init(tx), nil
}

func (h *spendLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	return h.assemble(ctx, tx, req, spendLockOperation, tx.Params.(*types.SpendLockParams).LockId)
}

func (h *spendLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *spendLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	return h.prepare(ctx, tx, req, spendLockOperation, tx.Params.(*types.SpendLockParams).LockId)
}
