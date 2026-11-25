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

package common

import (
	"context"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// ParseTransactionIDBytes32 parses a Paladin transaction id string as 32-byte hex or as a UUID (first 16 bytes of txId).
func ParseTransactionIDBytes32(ctx context.Context, id string) (pldtypes.Bytes32, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return pldtypes.Bytes32{}, i18n.NewError(ctx, msgs.MsgErrorParseTxId, "empty transaction id")
	}
	if b32, err := pldtypes.ParseBytes32Ctx(ctx, id); err == nil {
		return b32, nil
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return pldtypes.Bytes32{}, i18n.WrapError(ctx, err, msgs.MsgErrorParseTxId)
	}
	return pldtypes.Bytes32UUIDFirst16(u), nil
}
