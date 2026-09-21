/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The generation ordinal is Paladin's own and is decoupled from upstream semver, so the mapping is asserted here as
// well as documented: a silent drift between this table and domains/zeto/build.gradle zetoVersions would send the
// prover to the wrong artifact tree.
func TestZetoReleaseGenerationUpstreamMapping(t *testing.T) {
	assert.Equal(t, "v0.2.2", ZetoRelease_V0.UpstreamTag())
	assert.Equal(t, "v0.5.1", ZetoRelease_V1.UpstreamTag())

	for _, g := range SupportedZetoReleaseGenerations {
		assert.NotEmpty(t, g.UpstreamTag(), "generation %s has no upstream tag", g)
	}
}

func TestZetoReleaseGenerationArtifactDirName(t *testing.T) {
	assert.Equal(t, "V0", ZetoRelease_V0.ArtifactDirName())
	assert.Equal(t, "V1", ZetoRelease_V1.ArtifactDirName())

	// Directory names must be unique, or two generations would share a proving-artifact tree.
	seen := map[string]bool{}
	for _, g := range SupportedZetoReleaseGenerations {
		require.False(t, seen[g.ArtifactDirName()], "duplicate artifact dir %s", g.ArtifactDirName())
		seen[g.ArtifactDirName()] = true
	}
}

func TestValidateZetoReleaseGeneration(t *testing.T) {
	ctx := context.Background()
	for _, g := range SupportedZetoReleaseGenerations {
		require.NoError(t, ValidateZetoReleaseGeneration(ctx, g))
	}
	err := ValidateZetoReleaseGeneration(ctx, ZetoReleaseGeneration(99))
	require.Error(t, err)
	assert.Regexp(t, "PD210146", err)
}

func TestValidateZetoFungibleABIVersion(t *testing.T) {
	ctx := context.Background()
	for _, v := range SupportedZetoFungibleABIVersions {
		require.NoError(t, ValidateZetoFungibleABIVersion(ctx, v))
	}
	err := ValidateZetoFungibleABIVersion(ctx, ZetoFungibleABIVersion(99))
	require.Error(t, err)
	assert.Regexp(t, "PD210159", err)
}

// The two axes are independent: a pool may run Paladin's V1 transaction API against either upstream generation, so
// neither validation may reject a value on account of the other.
func TestVersionAxesAreIndependent(t *testing.T) {
	ctx := context.Background()
	for _, v := range SupportedZetoFungibleABIVersions {
		for _, g := range SupportedZetoReleaseGenerations {
			require.NoError(t, ValidateZetoFungibleABIVersion(ctx, v))
			require.NoError(t, ValidateZetoReleaseGeneration(ctx, g))
		}
	}
}

func TestZetoFungibleABIForVersion(t *testing.T) {
	// every supported version resolves to a distinct, non-empty ABI
	v0 := ZetoFungibleABIForVersion(ZetoFungibleABI_V0)
	v1 := ZetoFungibleABIForVersion(ZetoFungibleABI_V1)
	require.NotEmpty(t, v0)
	require.NotEmpty(t, v1)
	// V1 added the lock lifecycle methods; V0 has the legacy lock()
	assert.NotNil(t, v1.Functions()["createLock"])
	assert.Nil(t, v0.Functions()["createLock"])
	assert.NotNil(t, v0.Functions()["lock"])

	// an unknown version falls back to V0 rather than returning nil
	assert.Equal(t, v0, ZetoFungibleABIForVersion(ZetoFungibleABIVersion(99)))
}

func TestFungibleABIVersionRoundTrip(t *testing.T) {
	assert.Equal(t, ZetoFungibleABI_V1, FungibleABIVersion(1))
	assert.Equal(t, uint64(1), ZetoFungibleABI_V1.Uint64())
	assert.Equal(t, "V1", ZetoFungibleABI_V1.String())
	assert.Equal(t, "V0", ZetoRelease_V0.String())
}
