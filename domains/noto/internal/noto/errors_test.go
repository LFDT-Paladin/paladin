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

package noto

import (
	"errors"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInternalErrRevertsOnNeitherPhase(t *testing.T) {
	err := internalErr(fmt.Errorf("pop"))
	assert.False(t, err.IsAssembleRevert())
	assert.False(t, err.IsEndorseRevert())
}

func TestRevertRevertsOnBothPhases(t *testing.T) {
	err := revert(fmt.Errorf("pop"))
	assert.True(t, err.IsAssembleRevert())
	assert.True(t, err.IsEndorseRevert())
}

func TestEndorseOnlyRevertRevertsAtEndorseButNotAssemble(t *testing.T) {
	err := endorseOnlyRevert(fmt.Errorf("pop"))
	assert.False(t, err.IsAssembleRevert())
	assert.True(t, err.IsEndorseRevert())
}

func TestClassifiedErrorCarriesItsCause(t *testing.T) {
	cause := fmt.Errorf("pop")
	err := revert(cause)
	assert.Equal(t, "pop", err.Error())
	assert.True(t, errors.Is(err, cause))
}

func TestClassifyingANilCauseGivesANilInterface(t *testing.T) {
	assert.Nil(t, internalErr(nil))
	assert.Nil(t, revert(nil))
	assert.Nil(t, endorseOnlyRevert(nil))
}

func TestAssembleRevertOrError(t *testing.T) {
	res, err := assembleRevertOrError(nil)
	assert.Nil(t, res)
	assert.NoError(t, err)

	res, err = assembleRevertOrError(revert(fmt.Errorf("invalid")))
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
	assert.Equal(t, "invalid", *res.RevertReason)

	res, err = assembleRevertOrError(internalErr(fmt.Errorf("pop")))
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)

	res, err = assembleRevertOrError(endorseOnlyRevert(fmt.Errorf("pop")))
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)
}

func TestEndorseRevertOrError(t *testing.T) {
	res, err := endorseRevertOrError(nil)
	assert.Nil(t, res)
	assert.NoError(t, err)

	res, err = endorseRevertOrError(revert(fmt.Errorf("invalid")))
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.EndorseTransactionResponse_REVERT, res.EndorsementResult)
	assert.Equal(t, "invalid", *res.RevertReason)

	res, err = endorseRevertOrError(endorseOnlyRevert(fmt.Errorf("sender's fault")))
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.EndorseTransactionResponse_REVERT, res.EndorsementResult)
	assert.Equal(t, "sender's fault", *res.RevertReason)

	res, err = endorseRevertOrError(internalErr(fmt.Errorf("pop")))
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)
}

func assertRevert(t *testing.T, err NotoDomainError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.True(t, err.IsAssembleRevert(), "expected a revert at assemble")
	assert.True(t, err.IsEndorseRevert(), "expected a revert at endorse")
}

func assertEndorseOnlyRevert(t *testing.T, err NotoDomainError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.False(t, err.IsAssembleRevert(), "expected an internal error at assemble")
	assert.True(t, err.IsEndorseRevert(), "expected a revert at endorse")
}

func assertInternal(t *testing.T, err NotoDomainError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.False(t, err.IsAssembleRevert(), "expected no revert at assemble")
	assert.False(t, err.IsEndorseRevert(), "expected no revert at endorse")
}
