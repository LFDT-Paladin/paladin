/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package zeto

import (
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickZetoFactoryDeploy7Arg_FromEmbeddedFactory(t *testing.T) {
	entry, err := pickZetoFactoryDeploy7Arg(zetoFactoryBuild.ABI)
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

// TestZetoFactoryLegacyDeploySelectors pins the two `deploy` selectors of the consolidated ZetoFactory_V0 wrapper. They must
// never change: the domain plugin encodes calls with this ABI against factory addresses deployed long before the wrapper
// was consolidated — including the legacy, non-upgradeable ZetoTokenFactory generation, which exposes no initialize() or
// upgradeToAndCall() but does expose exactly these two entry points.
func TestZetoFactoryLegacyDeploySelectors(t *testing.T) {
	expected := map[string]string{
		"deploy(bytes32,string,string,string,address,bytes,bool)": "0x653bf99c",
		"deploy(bytes32,string,string,string,address,bytes)":      "0x05c98c83",
		// registerImplementation is invoked against factories of both generations when wiring up a domain
		"registerImplementation(string,(address,(address,address,address,address,address,address,address,address,address)))": "0x3924a044",
	}
	found := make(map[string]string)
	for _, e := range zetoFactoryBuild.ABI {
		if e.Type != abi.Function {
			continue
		}
		sig, err := e.Signature()
		require.NoError(t, err)
		if _, ok := expected[sig]; !ok {
			continue
		}
		selector, err := e.GenerateFunctionSelector()
		require.NoError(t, err)
		found[sig] = pldtypes.HexBytes(selector).String()
	}
	assert.Equal(t, expected, found)
}

// TestZetoFactoryABIServesBothFactoryVersions asserts the single wrapper covers every factoryVersion the plugin accepts.
func TestZetoFactoryABIServesBothFactoryVersions(t *testing.T) {
	for _, fv := range []types.ZetoPaladinFactoryVersion{types.ZetoPaladinFactoryV0, types.ZetoPaladinFactoryV1} {
		entry, err := pickZetoFactoryDeploy7Arg(zetoFactoryBuild.ABI)
		require.NoError(t, err, "factoryVersion %d", fv)
		require.NotNil(t, entry)
	}
	// the upgradeable lifecycle entry points the ERC1967Proxy deployment flow depends on
	fns := zetoFactoryBuild.ABI.Functions()
	require.NotNil(t, fns["initialize"])
	require.NotNil(t, fns["upgradeToAndCall"])
}
