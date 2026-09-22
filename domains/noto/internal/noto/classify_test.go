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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnkeyedErrorsAreInternal(t *testing.T) {
	assertInternalAssemble(t, fmt.Errorf("pop"), "pop")
	assertInternalEndorse(t, fmt.Errorf("pop"), "pop")
}

func TestForeignKeyedErrorsAreInternal(t *testing.T) {
	_, err := pldtypes.PrivateIdentityLocator("a@b@c").FullyQualified(t.Context(), "node1")
	require.Error(t, err)
	var pdErr i18n.PDError
	require.ErrorAs(t, err, &pdErr)
	assertInternalAssemble(t, err, string(pdErr.MessageKey()))
	assertInternalEndorse(t, err, string(pdErr.MessageKey()))
}

func TestClassificationSurvivesWrappingWithoutAKey(t *testing.T) {
	err := fmt.Errorf("outer: %w", i18n.NewError(t.Context(), msgs.MsgStateWrongOwner, "s1", "alice"))
	assertRevertAssemble(t, err, "outer: PD200018")
	assertRevertEndorse(t, err, "outer: PD200018")
}

func TestOutermostKeyWins(t *testing.T) {
	cause := i18n.NewError(t.Context(), msgs.MsgInsufficientFunds, "1")
	assertRevertAssemble(t, cause, "PD200005")
	assertRevertEndorse(t, cause, "PD200005")

	wrapped := i18n.WrapError(t.Context(), cause, msgs.MsgErrorVerifyingAddress, "notary")
	assertInternalAssemble(t, wrapped, "PD200011.*PD200005")
	assertRevertEndorse(t, wrapped, "PD200011.*PD200005")

	wrapped = i18n.WrapError(t.Context(), cause, msgs.MsgMissingNullifierSpec, 0)
	assertInternalAssemble(t, wrapped, "PD200046.*PD200005")
	assertInternalEndorse(t, wrapped, "PD200046.*PD200005")
}

func TestRevertTable(t *testing.T) {
	for key, row := range reverts {
		assert.False(t, row.assemble && !row.endorse, "%s reverts at assemble but not at endorse", key)
		assert.True(t, row.assemble || row.endorse, "%s reverts in neither phase and should have no row", key)
		err := i18n.NewError(t.Context(), key)
		if row.assemble {
			assertRevertAssemble(t, err, string(key))
		} else {
			assertInternalAssemble(t, err, string(key))
		}
		assertRevertEndorse(t, err, string(key))
	}
}

func TestAssembleRevertOrError(t *testing.T) {
	res, err := assembleRevertOrError(nil)
	assert.Nil(t, res)
	assert.NoError(t, err)

	res, err = assembleRevertOrError(i18n.NewError(t.Context(), msgs.MsgInvalidLockTransition))
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
	assert.Regexp(t, "PD200041", *res.RevertReason)

	res, err = assembleRevertOrError(fmt.Errorf("pop"))
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)

	res, err = assembleRevertOrError(i18n.NewError(t.Context(), msgs.MsgErrorVerifyingAddress, "notary"))
	assert.Nil(t, res)
	assert.Regexp(t, "PD200011", err)
}

func TestEndorseRevertOrError(t *testing.T) {
	res, err := endorseRevertOrError(nil)
	assert.Nil(t, res)
	assert.NoError(t, err)

	res, err = endorseRevertOrError(i18n.NewError(t.Context(), msgs.MsgInvalidLockTransition))
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.EndorseTransactionResponse_REVERT, res.EndorsementResult)
	assert.Regexp(t, "PD200041", *res.RevertReason)

	res, err = endorseRevertOrError(i18n.NewError(t.Context(), msgs.MsgErrorVerifyingAddress, "notary"))
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, prototk.EndorseTransactionResponse_REVERT, res.EndorsementResult)
	assert.Regexp(t, "PD200011", *res.RevertReason)

	res, err = endorseRevertOrError(fmt.Errorf("pop"))
	assert.Nil(t, res)
	assert.Regexp(t, "pop", err)
}

func assertRevertAssemble(t *testing.T, err error, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.True(t, isAssembleRevert(err), "expected a revert at assemble")
}

func assertRevertEndorse(t *testing.T, err error, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.True(t, isEndorseRevert(err), "expected a revert at endorse")
}

func assertInternalAssemble(t *testing.T, err error, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.False(t, isAssembleRevert(err), "expected an internal error at assemble")
}

func assertInternalEndorse(t *testing.T, err error, match string) {
	t.Helper()
	require.Error(t, err)
	require.Regexp(t, match, err)
	assert.False(t, isEndorseRevert(err), "expected an internal error at endorse")
}
