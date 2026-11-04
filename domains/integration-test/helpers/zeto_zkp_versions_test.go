/*
 * Copyright © 2024 Kaleido, Inc.
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

package helpers

import (
	"os"
	"path/filepath"
	"testing"

	zetotypes "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZetoZKArtifactsDir(t *testing.T) {
	assert.Equal(t, filepath.Join("..", "zeto", "zkp", "v0.2.2"), ZetoZKArtifactsDir(zetotypes.ZetoVariant_V0))
	assert.Equal(t, filepath.Join("..", "zeto", "zkp", "v0.5.1"), ZetoZKArtifactsDir(zetotypes.ZetoVariant_V1))
}

func TestZetoZKArtifactRootCoversSupportedVariants(t *testing.T) {
	for _, v := range zetotypes.SupportedZetoVariants {
		assert.NotEmpty(t, zetoZKArtifactRoot[v], "variant %s has no artifact root", v)
	}
}

func TestZetoVariantArtifactsDir(t *testing.T) {
	dir := ZetoVariantArtifactsDir(t, zetotypes.ZetoVariant_V0, zetotypes.ZetoVariant_V1)
	for _, v := range []zetotypes.ZetoVariant{zetotypes.ZetoVariant_V0, zetotypes.ZetoVariant_V1} {
		target, err := os.Readlink(filepath.Join(dir, v.String()))
		require.NoError(t, err)
		expected, err := filepath.Abs(ZetoZKArtifactsDir(v))
		require.NoError(t, err)
		assert.Equal(t, expected, target)
	}
}

func TestResolveZetoImplementationAbiPath_passThrough(t *testing.T) {
	assert.Equal(t, "/abs/foo.json", ResolveZetoImplementationAbiPath("/abs/foo.json", zetotypes.ZetoVariant_V0))
	assert.Equal(t, "./contracts/Foo.json", ResolveZetoImplementationAbiPath("./contracts/Foo.json", zetotypes.ZetoVariant_V0))
}

func TestResolveZetoImplementationAbiPath_alreadyVersioned(t *testing.T) {
	p := "./helpers/abis/zkp/v0.2.2/Zeto_Anon.json"
	assert.Equal(t, p, ResolveZetoImplementationAbiPath(p, zetotypes.ZetoVariant_V1))
}

func TestResolveZetoImplementationAbiPath_prefersZkpTreeWhenPresent(t *testing.T) {
	t.Chdir(t.TempDir())

	require.NoError(t, os.MkdirAll(filepath.Join("helpers", "abis", "zkp", "v0.5.1"), 0o755))
	versioned := filepath.Join("helpers", "abis", "zkp", "v0.5.1", "Zeto_Anon.json")
	require.NoError(t, os.WriteFile(versioned, []byte(`{"abi":[]}`), 0o644))

	assert.Equal(t, versioned, ResolveZetoImplementationAbiPath("./helpers/abis/Zeto_Anon.json", zetotypes.ZetoVariant_V1))
	assert.Equal(t, "./helpers/abis/Zeto_Anon.json", ResolveZetoImplementationAbiPath("./helpers/abis/Zeto_Anon.json", zetotypes.ZetoVariant_V0))
}
