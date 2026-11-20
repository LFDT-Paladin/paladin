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

// variantDirName is the per-variant artifact subdirectory, matching ZetoVariant.ArtifactDirName().
// It is duplicated here rather than imported to keep the signer free of a dependency on the domain types package.
func variantDirName(variant uint64) string {
	return fmt.Sprintf("V%d", variant)
}

// loadCircuit reads the circuit's artifacts from <dir>/<variantDir>/, where variantDir is empty for a prover whose
// artifacts are not laid out per variant.
func loadCircuit(ctx context.Context, variantDir string, circuitName string, config *zetosignerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
	if config.CircuitsDir == "" {
		return nil, []byte{}, i18n.NewError(ctx, msgs.MsgInvalidConfigCircuitRoot)
	}
	if config.ProvingKeysDir == "" {
		return nil, []byte{}, i18n.NewError(ctx, msgs.MsgInvalidConfigProvingKeysRoot)
	}

	// load the wasm file for the circuit
	wasmBytes, err := os.ReadFile(path.Join(config.CircuitsDir, variantDir, fmt.Sprintf("%s_js", circuitName), fmt.Sprintf("%s.wasm", circuitName)))
	if err != nil {
		return nil, []byte{}, err
	}

	// create the prover
	zkeyBytes, err := os.ReadFile(path.Join(config.ProvingKeysDir, variantDir, fmt.Sprintf("%s.zkey", circuitName)))
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
