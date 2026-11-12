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

package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	zetotypes "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/stretchr/testify/require"
)

// zetoZKArtifactRoot is the directory under domains/zeto/zkp/ that each variant's release is extracted to: the zkpRoot of
// the matching zetoVersions entry in domains/zeto/build.gradle.
var zetoZKArtifactRoot = map[zetotypes.ZetoVariant]string{
	zetotypes.ZetoVariant_V0: "v0.2.2",
	zetotypes.ZetoVariant_V1: "v0.5.1",
}

// ZetoZKArtifactsDir returns a variant's circuits/proving-keys root for integration tests, laid out flat
// (cwd = domains/integration-test).
func ZetoZKArtifactsDir(variant zetotypes.ZetoVariant) string {
	return filepath.Join("..", "zeto", "zkp", zetoZKArtifactRoot[variant])
}

// ZetoVariantArtifactsDir returns a directory holding each variant's artifacts in a V<n>/ subdirectory, the layout a
// domain configured with variants reads.
func ZetoVariantArtifactsDir(t *testing.T, variants ...zetotypes.ZetoVariant) string {
	dir := t.TempDir()
	for _, v := range variants {
		src, err := filepath.Abs(ZetoZKArtifactsDir(v))
		require.NoError(t, err)
		require.NoError(t, os.Symlink(src, filepath.Join(dir, v.String())))
	}
	return dir
}

// ResolveZetoImplementationAbiPath maps legacy flat helpers/abis/<name>.json paths from older deploy YAML
// to helpers/abis/zkp/<zkpRoot>/<name>.json for the variant's zkpRoot when that file exists (upstream artifacts reuse
// basenames across zeto releases). Paths already under helpers/abis/zkp/<version>/ are returned unchanged. Paladin-compiled
// factory JSON (e.g. ZetoFactory_V1.json) stays under helpers/abis/ and keeps the original path when no zkp copy exists.
func ResolveZetoImplementationAbiPath(configPath string, variant zetotypes.ZetoVariant) string {
	clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(configPath)), "./")
	const flatPrefix = "helpers/abis/"
	if !strings.HasPrefix(clean, flatPrefix) {
		return configPath
	}
	afterAbis := strings.TrimPrefix(clean, flatPrefix)
	if strings.HasPrefix(afterAbis, "zkp/") {
		return configPath
	}
	base := filepath.Base(filepath.FromSlash(clean))
	candidate := filepath.Join("helpers", "abis", "zkp", zetoZKArtifactRoot[variant], base)
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return candidate
	}
	return configPath
}

// ZetoZKArtifactsPresent reports whether minimal anon circuit artifacts exist for a variant (cwd = domains/integration-test).
func ZetoZKArtifactsPresent(variant zetotypes.ZetoVariant) bool {
	base := ZetoZKArtifactsDir(variant)
	wasm := filepath.Join(base, "anon_js", "anon.wasm")
	key := filepath.Join(base, "anon.zkey")
	st, err := os.Stat(wasm)
	if err != nil || st.IsDir() {
		return false
	}
	st, err = os.Stat(key)
	return err == nil && !st.IsDir()
}
