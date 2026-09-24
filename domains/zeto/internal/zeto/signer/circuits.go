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

package signer

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/iden3/go-rapidsnark/witness/v2"
	"github.com/iden3/go-rapidsnark/witness/wasmer"
)

// generationDirName is the per-generation artifact subdirectory, matching ZetoReleaseGeneration.ArtifactDirName().
// It is duplicated here rather than imported to keep the signer free of a dependency on the domain types package.
func generationDirName(generation uint64) string {
	return fmt.Sprintf("V%d", generation)
}

// resolveArtifact returns the path to an artifact for a generation, preferring the per-generation layout
//
//	<root>/V<n>/<rel>
//
// and falling back to the legacy flat layout <root>/<rel> when the generation directory is not present. The fallback
// keeps nodes working across an upgrade where the mounted artifact volume has not been re-laid-out yet; a node that
// must serve more than one generation needs the per-generation layout, because circuit names collide across releases.
func resolveArtifact(root string, generation uint64, rel string) string {
	versioned := path.Join(root, generationDirName(generation), rel)
	if _, err := os.Stat(versioned); err == nil {
		return versioned
	}
	return path.Join(root, rel)
}

func loadCircuit(ctx context.Context, generation uint64, circuitName string, config *zetosignerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
	if config.CircuitsDir == "" {
		return nil, []byte{}, i18n.NewError(ctx, msgs.MsgInvalidConfigCircuitRoot)
	}
	if config.ProvingKeysDir == "" {
		return nil, []byte{}, i18n.NewError(ctx, msgs.MsgInvalidConfigProvingKeysRoot)
	}

	// load the wasm file for the circuit
	wasmRel := path.Join(fmt.Sprintf("%s_js", circuitName), fmt.Sprintf("%s.wasm", circuitName))
	wasmBytes, err := os.ReadFile(resolveArtifact(config.CircuitsDir, generation, wasmRel))
	if err != nil {
		return nil, []byte{}, err
	}

	// create the prover
	zkeyRel := fmt.Sprintf("%s.zkey", circuitName)
	zkeyBytes, err := os.ReadFile(resolveArtifact(config.ProvingKeysDir, generation, zkeyRel))
	if err != nil {
		return nil, []byte{}, err
	}

	// create the calculator
	var ops []witness.Option
	ops = append(ops, witness.WithWasmEngine(wasmer.NewCircom2WitnessCalculator))
	calc, err := witness.NewCalculator(wasmBytes, ops...)
	if err != nil {
		return nil, []byte{}, err
	}

	return calc, zkeyBytes, err
}

func getBatchCircuit(circuitId string) string {
	return fmt.Sprintf("%s_batch", circuitId)
}

func IsBatchCircuit(circuitId string) bool {
	return strings.HasSuffix(circuitId, "_batch")
}
