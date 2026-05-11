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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZetoZKArtifactsDir_latestAndEmpty(t *testing.T) {
	assert.Contains(t, ZetoZKArtifactsDir("latest"), ZetoZKArtifactRootLatest)
	assert.Contains(t, ZetoZKArtifactsDir(""), ZetoZKArtifactRootLatest)
	assert.Contains(t, ZetoZKArtifactsDir("v0.5.0"), "v0.5.0")
}

func TestZetoZKArtifactRootsForTestRun_matrix(t *testing.T) {
	t.Setenv(EnvZetoZKPMatrix, "")
	require.Len(t, ZetoZKArtifactRootsForTestRun(), 1)

	t.Setenv(EnvZetoZKPMatrix, "all")
	t.Setenv(EnvZetoZKPVersion, "")
	roots := ZetoZKArtifactRootsForTestRun()
	require.Len(t, roots, len(SupportedZetoZKArtifactRoots()))
	assert.Equal(t, SupportedZetoZKArtifactRoots(), roots)

	_ = os.Unsetenv(EnvZetoZKPMatrix)
}

func TestResolveZetoImplementationAbiPath_passThrough(t *testing.T) {
	assert.Equal(t, "/abs/foo.json", ResolveZetoImplementationAbiPath("/abs/foo.json", "v0.2.2"))
	assert.Equal(t, "./contracts/Foo.json", ResolveZetoImplementationAbiPath("./contracts/Foo.json", "v0.2.2"))
}

func TestResolveZetoImplementationAbiPath_alreadyVersioned(t *testing.T) {
	p := "./helpers/abis/zkp/v0.5.0/Zeto_Anon.json"
	assert.Equal(t, p, ResolveZetoImplementationAbiPath(p, "v0.5.0"))
}

func TestResolveZetoImplementationAbiPath_prefersZkpTreeWhenPresent(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))

	require.NoError(t, os.MkdirAll(filepath.Join("helpers", "abis", "zkp", "v9.9.9"), 0o755))
	versioned := filepath.Join("helpers", "abis", "zkp", "v9.9.9", "Zeto_Anon.json")
	require.NoError(t, os.WriteFile(versioned, []byte(`{"abi":[]}`), 0o644))

	resolved := ResolveZetoImplementationAbiPath("./helpers/abis/Zeto_Anon.json", "v9.9.9")
	assert.Equal(t, versioned, resolved)
}
