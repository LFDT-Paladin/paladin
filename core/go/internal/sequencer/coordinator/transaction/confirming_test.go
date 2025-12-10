/*
 * Copyright © 2025 Kaleido, Inc.
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
package transaction

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
)

func TestGuard_HasRevertReason_FalseWhenEmpty(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Initially revertReason should be nil (zero value for HexBytes)
	// When nil, String() returns "", so guard returns false
	assert.False(t, guard_HasRevertReason(ctx, txn))

	// Note: An empty slice HexBytes{} would return "0x" from String(),
	// which is not empty, so the guard would return true. Only nil returns false.
}

func TestGuard_HasRevertReason_TrueWhenSet(t *testing.T) {
	ctx := context.Background()
	txn, _ := newTransactionForUnitTesting(t, nil)

	// Set revertReason to a non-empty value
	txn.revertReason = pldtypes.MustParseHexBytes("0x1234567890abcdef")
	assert.True(t, guard_HasRevertReason(ctx, txn))

	// Test with another value
	txn.revertReason = pldtypes.MustParseHexBytes("0xdeadbeef")
	assert.True(t, guard_HasRevertReason(ctx, txn))
}

