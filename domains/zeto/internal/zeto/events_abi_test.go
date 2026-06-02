/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package zeto

import (
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZetoEventABISet_IncludesCoreAndLockableEvents(t *testing.T) {
	for _, v := range []types.ZetoTargetContractABIVersion{types.ZetoTargetContractABI_V0, types.ZetoTargetContractABI_V1} {
		events := zetoEventABISet(v)
		require.NotEmpty(t, events)
		names := make(map[string]struct{})
		for _, e := range events {
			require.Equal(t, abi.Event, e.Type)
			names[e.Name] = struct{}{}
		}
		assert.Contains(t, names, "UTXOTransfer")
		if v == types.ZetoTargetContractABI_V1 {
			assert.Contains(t, names, "ZetoLockCreated")
		}
	}
}

func TestGetAllZetoEventAbis_DedupesAcrossVersions(t *testing.T) {
	all := getAllZetoEventAbis()
	require.NotEmpty(t, all)
	seen := make(map[string]int)
	for _, e := range all {
		seen[e.Name]++
	}
	for name, count := range seen {
		assert.Equal(t, 1, count, "event %s should appear once after dedup", name)
	}
}

func TestDedupEvents(t *testing.T) {
	events := abi.ABI{
		{Type: abi.Event, Name: "A"},
		{Type: abi.Event, Name: "B"},
		{Type: abi.Event, Name: "A"},
	}
	deduped := dedupEvents(events)
	require.Len(t, deduped, 2)
}

func TestZetoCoreABIBytes_PanicsOnUnsupportedVersion(t *testing.T) {
	assert.Panics(t, func() {
		_ = zetoCoreABIBytes(99)
	})
}
