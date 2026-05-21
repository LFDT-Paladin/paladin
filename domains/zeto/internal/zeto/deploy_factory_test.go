/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package zeto

import (
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickZetoFactoryDeploy7Arg_FromEmbeddedFactory(t *testing.T) {
	entry, err := pickZetoFactoryDeploy7Arg(zetoFactoryBuildV0.ABI)
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "deploy", entry.Name)
	require.Len(t, entry.Inputs, 7)
}

func TestPickZetoFactoryDeploy7Arg_Errors(t *testing.T) {
	_, err := pickZetoFactoryDeploy7Arg(abi.ABI{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "7-argument deploy")

	sevenInputs := abi.ParameterArray{
		{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"}, {Name: "f"}, {Name: "g"},
	}
	duplicate := abi.ABI{
		{Type: abi.Function, Name: "deploy", Inputs: sevenInputs},
		{Type: abi.Function, Name: "deploy", Inputs: sevenInputs},
	}
	_, err = pickZetoFactoryDeploy7Arg(duplicate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple 7-argument deploy")
}
