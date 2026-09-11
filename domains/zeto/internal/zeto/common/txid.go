/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

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
