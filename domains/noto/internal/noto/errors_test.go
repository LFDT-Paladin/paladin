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

func TestClassificationsSelectTheIntendedOutcome(t *testing.T) {
	cause := fmt.Errorf("pop")

	// Nobody's fault but ours: retried on both phases, never terminally failing a
	// transaction that would have succeeded on the next attempt
	internal := internalErr(cause)
	assert.False(t, internal.IsAssembleRevert())
	assert.False(t, internal.IsEndorseRevert())

	// Invalid on any path: the transaction is failed immediately and the reason reported
	validation := validationErr(cause)
	assert.True(t, validation.IsAssembleRevert())
	assert.True(t, validation.IsEndorseRevert())

	// Invalid only once the data has crossed the wire from the coordinator
	endorseOnly := endorseOnlyValidationErr(cause)
	assert.False(t, endorseOnly.IsAssembleRevert())
	assert.True(t, endorseOnly.IsEndorseRevert())
}

func TestClassifiedErrorCarriesItsCause(t *testing.T) {
	cause := fmt.Errorf("pop")
	err := validationErr(cause)
	assert.Equal(t, "pop", err.Error())
	assert.True(t, errors.Is(err, cause))
}

func TestClassifyingNoErrorGivesNoError(t *testing.T) {
	// The constructors are applied to errors that may or may not be set, so a nil cause has
	// to give a nil interface rather than a non-nil NotoDomainError wrapping nothing
	assert.Nil(t, internalErr(nil))
	assert.Nil(t, validationErr(nil))
	assert.Nil(t, endorseOnlyValidationErr(nil))
}

func TestAssembleRevertOrError(t *testing.T) {
	res, err := assembleRevertOrError(nil)
	assert.Nil(t, res)
	assert.NoError(t, err)

	res, err = assembleRevertOrError(validationErr(fmt.Errorf("invalid")))
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
	assert.Equal(t, "invalid", *res.RevertReason)

	res, err = assembleRevertOrError(internalErr(fmt.Errorf("pop")))
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)

	// The endorse-only classification errors here, so the platform retries on the originator
	res, err = assembleRevertOrError(endorseOnlyValidationErr(fmt.Errorf("pop")))
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)
}

func TestEndorseRevertOrError(t *testing.T) {
	res, err := endorseRevertOrError(nil)
	assert.Nil(t, res)
	assert.NoError(t, err)

	res, err = endorseRevertOrError(validationErr(fmt.Errorf("invalid")))
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.EndorseTransactionResponse_REVERT, res.EndorsementResult)
	assert.Equal(t, "invalid", *res.RevertReason)

	res, err = endorseRevertOrError(endorseOnlyValidationErr(fmt.Errorf("sender's fault")))
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.EndorseTransactionResponse_REVERT, res.EndorsementResult)
	assert.Equal(t, "sender's fault", *res.RevertReason)

	res, err = endorseRevertOrError(internalErr(fmt.Errorf("pop")))
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)
}

// assertRevert: the transaction is invalid wherever it is seen, so both phases reject it.
func assertRevert(t *testing.T, err NotoDomainError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.True(t, err.IsAssembleRevert(), "expected a revert at assemble")
	assert.True(t, err.IsEndorseRevert(), "expected a revert at endorse")
}

// assertEndorseOnlyRevert: at assemble the failing value was generated on this node, so it can
// only mean a defect here; at endorse the same value was chosen by the originator.
func assertEndorseOnlyRevert(t *testing.T, err NotoDomainError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.False(t, err.IsAssembleRevert(), "expected an internal error at assemble")
	assert.True(t, err.IsEndorseRevert(), "expected a revert at endorse")
}

// assertInternal: nothing about the transaction is wrong, so it stays retryable in both phases.
func assertInternal(t *testing.T, err NotoDomainError, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.False(t, err.IsAssembleRevert(), "expected no revert at assemble")
	assert.False(t, err.IsEndorseRevert(), "expected no revert at endorse")
}
