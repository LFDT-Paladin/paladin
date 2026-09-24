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

package zetosignerapi

import (
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/signerapi"
)

// StaticKeyEntryConfig is the configuration for a ZK prover
// based on SNARK, which typically takes a circuit and proving key
type SnarkProverConfig struct {
	signerapi.ConfigNoExt
	// CircuitsDir and ProvingKeysDir are the roots of the proving-artifact trees. Artifacts are looked up under a
	// per-generation subdirectory first — <dir>/V<n>/<circuit>_js/<circuit>.wasm and <dir>/V<n>/<circuit>.zkey, where
	// V<n> is the pool's ZetoReleaseGeneration — so one node can serve pools of every generation. Circuit names are
	// not unique across upstream zeto releases, which is why the generation cannot be left out of the path.
	//
	// A root laid out flat (no V<n> directories) still resolves, for nodes pinned to a single generation and for
	// artifact volumes that predate this layout; such a node can only serve the generation it holds.
	CircuitsDir         string `json:"circuitsDir"`         // directory for the circuits runtime (WASM currently supported)
	ProvingKeysDir      string `json:"provingKeysDir"`      // public parameters for the prover, specific to each circuit
	MaxProverPerCircuit *int   `json:"maxProverPerCircuit"` // maximum number of proving runtime per circuit, each prover owns a standalone WASM instance
}

type CircuitType string

const (
	Deposit        CircuitType = "deposit"
	Withdraw       CircuitType = "withdraw"
	Transfer       CircuitType = "transfer"
	TransferLocked CircuitType = "transferLocked"
)

type Circuit struct {
	Name           string      `yaml:"name" json:"name"`
	Type           CircuitType `yaml:"type" json:"type"`
	UsesNullifiers bool        `yaml:"usesNullifiers" json:"usesNullifiers"`
	UsesEncryption bool        `yaml:"usesEncryption" json:"usesEncryption"`
	UsesKyc        bool        `yaml:"usesKyc" json:"usesKyc"`
	// Generation is the pool's ZetoReleaseGeneration, stamped when the on-chain domain config is decoded. It selects
	// which proving-artifact tree this circuit's .wasm and .zkey are read from; 0 (V0) is also the legacy flat layout.
	Generation uint64 `yaml:"generation,omitempty" json:"generation,omitempty"`
}

func (c *Circuit) ToProto() *proto.Circuit {
	return &proto.Circuit{
		Name:           c.Name,
		Type:           string(c.Type),
		UsesNullifiers: c.UsesNullifiers,
		UsesEncryption: c.UsesEncryption,
		UsesKyc:        c.UsesKyc,
		Generation:     c.Generation,
	}
}

type Circuits map[string]*Circuit

func (cs Circuits) Init() {
	for circuitType, circuit := range cs {
		circuit.Type = CircuitType(circuitType)
	}
}

// StampGeneration records the pool's release generation on every circuit, so the prover reads artifacts from that
// generation's tree. Called when the on-chain domain instance config is decoded.
func (cs Circuits) StampGeneration(generation uint64) {
	for _, circuit := range cs {
		if circuit != nil {
			circuit.Generation = generation
		}
	}
}

func NewCircuitFromProto(pb *proto.Circuit) *Circuit {
	return &Circuit{
		Name:           pb.Name,
		Type:           CircuitType(pb.Type),
		UsesNullifiers: pb.UsesNullifiers,
		UsesEncryption: pb.UsesEncryption,
		UsesKyc:        pb.UsesKyc,
		Generation:     pb.Generation,
	}
}

// Implements the extensible config interface of the signer
var _ signerapi.ExtensibleConfig = &SnarkProverConfig{}

func (c *SnarkProverConfig) KeyStoreConfig() *pldconf.KeyStoreConfig {
	return &c.KeyStore
}

func (c *SnarkProverConfig) KeyDerivationConfig() *pldconf.KeyDerivationConfig {
	return &c.KeyDerivation
}
