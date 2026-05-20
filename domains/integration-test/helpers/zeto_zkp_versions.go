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
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ZKP artifact directory names under domains/zeto/zkp/ — keep aligned with domains/zeto/build.gradle zetoVersions[].zkpRoot.
const (
	ZetoZKArtifactRootLatest = "v0.2.2"
	ZetoZKArtifactRootV050   = "v0.5.0"
)

// EnvZetoZKPVersion selects a single zkp root (e.g. "v0.5.0"). Empty uses ZetoZKArtifactRootLatest.
const EnvZetoZKPVersion = "PALADIN_ZETO_ZKP_VERSION"

// EnvZetoZKPMatrix when set to "all" runs integration suites once per SupportedZetoZKArtifactRoots (skipping roots with missing artifacts).
const EnvZetoZKPMatrix = "PALADIN_ZETO_ZKP_MATRIX"

// SupportedZetoZKArtifactRoots lists every zeto-contracts / wasm / proving-keys tree Gradle may extract.
func SupportedZetoZKArtifactRoots() []string {
	return []string{ZetoZKArtifactRootLatest, ZetoZKArtifactRootV050}
}

// EffectiveZetoZKArtifactRoot returns the zkp subdirectory name for a single-version test run.
func EffectiveZetoZKArtifactRoot() string {
	v := strings.TrimSpace(os.Getenv(EnvZetoZKPVersion))
	if v == "" || strings.EqualFold(v, "latest") {
		return ZetoZKArtifactRootLatest
	}
	if err := validateZetoZKArtifactRoot(v); err != nil {
		panic(err) // misconfigured CI / developer env
	}
	return v
}

// ZetoZKArtifactRootsForTestRun returns one root (EffectiveZetoZKArtifactRoot) or all supported roots when PALADIN_ZETO_ZKP_MATRIX=all.
func ZetoZKArtifactRootsForTestRun() []string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(EnvZetoZKPMatrix)), "all") {
		out := make([]string, len(SupportedZetoZKArtifactRoots()))
		copy(out, SupportedZetoZKArtifactRoots())
		return out
	}
	return []string{EffectiveZetoZKArtifactRoot()}
}

// ZetoV050ZKArtifactRootsForTestRun returns the v0.5.0 zkp root for TestFungibleZetoV050Suite subtests (t.Run(root, ...)).
func ZetoV050ZKArtifactRootsForTestRun() []string {
	return []string{ZetoZKArtifactRootV050}
}

func validateZetoZKArtifactRoot(v string) error {
	for _, allowed := range SupportedZetoZKArtifactRoots() {
		if v == allowed {
			return nil
		}
	}
	return fmt.Errorf("%s=%q is not a supported zkp root; allowed: %v", EnvZetoZKPVersion, v, SupportedZetoZKArtifactRoots())
}

// ResolveZetoImplementationAbiPath maps legacy flat helpers/abis/<name>.json paths from older deploy YAML
// to helpers/abis/zkp/<zkpRoot>/<name>.json when that file exists (upstream artifacts reuse basenames across
// zeto releases). Paths already under helpers/abis/zkp/<version>/ are returned unchanged. Paladin-compiled
// factory JSON (e.g. ZetoFactory.json) stays under helpers/abis/ and keeps the original path when no zkp copy exists.
func ResolveZetoImplementationAbiPath(configPath, zkpRoot string) string {
	zkpRoot = strings.TrimSpace(zkpRoot)
	if zkpRoot == "" {
		zkpRoot = EffectiveZetoZKArtifactRoot()
	}
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
	candidate := filepath.Join("helpers", "abis", "zkp", zkpRoot, base)
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return candidate
	}
	return configPath
}

// ZetoZKArtifactsRootPresent reports whether minimal anon circuit artifacts exist for root (cwd = domains/integration-test).
func ZetoZKArtifactsRootPresent(root string) bool {
	base := ZetoZKArtifactsDir(root)
	wasm := filepath.Join(base, "anon_js", "anon.wasm")
	key := filepath.Join(base, "anon.zkey")
	st, err := os.Stat(wasm)
	if err != nil || st.IsDir() {
		return false
	}
	st, err = os.Stat(key)
	return err == nil && !st.IsDir()
}
