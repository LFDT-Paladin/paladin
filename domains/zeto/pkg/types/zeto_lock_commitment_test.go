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

package types

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeZetoSpendCommitment_EmptySpendPath(t *testing.T) {
	ctx := context.Background()
	lockedInputs := []string{"0x00000000000000000000000000000000000000000000000000000000000000aa"}
	spendData := []byte(`[{"to":"alice@node","amount":"1"}]`)

	hash1, err := ComputeZetoSpendCommitment(ctx, lockedInputs, []string{}, []string{"0xbb"}, spendData)
	require.NoError(t, err)
	hash2, err := ComputeZetoSpendCommitment(ctx, lockedInputs, nil, []string{"0xbb"}, spendData)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)
	assert.False(t, hash1.IsZero())

	cancelHash, err := ComputeZetoCancelCommitment(ctx, lockedInputs, []string{}, []string{"0xcc"}, spendData)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, cancelHash, "spend and cancel domains must differ")
}

func TestComputeZetoUnlockCommitment_DomainSeparation(t *testing.T) {
	ctx := context.Background()
	lockedInputs := []string{"0x01"}
	outputs := []string{"0x02"}
	data := []byte{0xab}

	spend, err := ComputeZetoUnlockCommitment(ctx, ZetoSpendCommitmentDomain, lockedInputs, []string{}, outputs, data)
	require.NoError(t, err)
	cancel, err := ComputeZetoUnlockCommitment(ctx, ZetoCancelCommitmentDomain, lockedInputs, []string{}, outputs, data)
	require.NoError(t, err)
	assert.NotEqual(t, spend, cancel)
}
