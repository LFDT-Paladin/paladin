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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorClassification(t *testing.T) {
	cause := fmt.Errorf("pop")

	t.Run("both stages", func(t *testing.T) {
		for _, tc := range []struct{ assembleRevert, endorseRevert bool }{{false, false}, {true, true}, {false, true}} {
			var err AssembleOrEndorseError = assembleOrEndorseError{cause, tc.assembleRevert, tc.endorseRevert}
			assert.Equal(t, tc.assembleRevert, err.IsAssembleRevert())
			assert.Equal(t, tc.endorseRevert, err.IsEndorseRevert())
			assert.Equal(t, "pop", err.Error())
		}
	})

	t.Run("assemble only", func(t *testing.T) {
		for _, revert := range []bool{false, true} {
			var err AssembleError = assembleError{cause, revert}
			assert.Equal(t, revert, err.IsAssembleRevert())
			assert.Equal(t, "pop", err.Error())
			_, answersEndorse := any(err).(EndorseError)
			assert.False(t, answersEndorse, "must not answer for a stage that cannot reach it")
		}
	})

	t.Run("endorse only", func(t *testing.T) {
		for _, revert := range []bool{false, true} {
			var err EndorseError = endorseError{cause, revert}
			assert.Equal(t, revert, err.IsEndorseRevert())
			assert.Equal(t, "pop", err.Error())
			_, answersAssemble := any(err).(AssembleError)
			assert.False(t, answersAssemble, "must not answer for a stage that cannot reach it")
		}
	})
}

func TestAssembleRevertOrError(t *testing.T) {
	res, err := assembleRevertOrError(nil)
	assert.Nil(t, res)
	assert.NoError(t, err)

	res, err = assembleRevertOrError(assembleOrEndorseError{fmt.Errorf("invalid"), true, true})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
	assert.Equal(t, "invalid", *res.RevertReason)

	res, err = assembleRevertOrError(assembleError{fmt.Errorf("broke"), true})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
	assert.Equal(t, "broke", *res.RevertReason)

	res, err = assembleRevertOrError(assembleError{fmt.Errorf("pop"), false})
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)

	res, err = assembleRevertOrError(assembleOrEndorseError{fmt.Errorf("pop"), false, true})
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)
}

func TestEndorseRevertOrError(t *testing.T) {
	res, err := endorseRevertOrError(nil)
	assert.Nil(t, res)
	assert.NoError(t, err)

	res, err = endorseRevertOrError(assembleOrEndorseError{fmt.Errorf("invalid"), true, true})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.EndorseTransactionResponse_REVERT, res.EndorsementResult)
	assert.Equal(t, "invalid", *res.RevertReason)

	res, err = endorseRevertOrError(endorseError{fmt.Errorf("forged"), true})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.EndorseTransactionResponse_REVERT, res.EndorsementResult)
	assert.Equal(t, "forged", *res.RevertReason)

	res, err = endorseRevertOrError(assembleOrEndorseError{fmt.Errorf("sender's fault"), false, true})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.EndorseTransactionResponse_REVERT, res.EndorsementResult)
	assert.Equal(t, "sender's fault", *res.RevertReason)

	res, err = endorseRevertOrError(endorseError{fmt.Errorf("pop"), false})
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)
}

// The helpers below take the interface of the function under test, so a test can only assert
// the stages that function is reachable from.
func assertRevert(t *testing.T, err AssembleOrEndorseError, match string) {
	t.Helper()
	assertAssembleRevert(t, err, match)
	assertEndorseRevert(t, err, match)
}

func assertEndorseOnlyRevert(t *testing.T, err AssembleOrEndorseError, match string) {
	t.Helper()
	assertAssembleInternal(t, err, match)
	assertEndorseRevert(t, err, match)
}

func assertAssembleRevert(t *testing.T, err AssembleError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.True(t, err.IsAssembleRevert(), "expected a revert at assemble")
}

func assertEndorseRevert(t *testing.T, err EndorseError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.True(t, err.IsEndorseRevert(), "expected a revert at endorse")
}

func assertAssembleInternal(t *testing.T, err AssembleError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.False(t, err.IsAssembleRevert(), "expected an internal error at assemble")
}

func assertEndorseInternal(t *testing.T, err EndorseError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.False(t, err.IsEndorseRevert(), "expected an internal error at endorse")
}
