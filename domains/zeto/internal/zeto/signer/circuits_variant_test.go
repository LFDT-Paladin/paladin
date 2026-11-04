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

package signer

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/signer/common"
	pb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// writeArtifacts lays down a circuit's .wasm and .zkey under root, optionally inside a variant directory.
func writeArtifacts(t *testing.T, root, variantDir, circuit, marker string) {
	t.Helper()
	base := path.Join(root, variantDir)
	require.NoError(t, os.MkdirAll(path.Join(base, circuit+"_js"), 0o755))
	require.NoError(t, os.WriteFile(path.Join(base, circuit+"_js", circuit+".wasm"), []byte("wasm-"+marker), 0o644))
	require.NoError(t, os.WriteFile(path.Join(base, circuit+".zkey"), []byte("zkey-"+marker), 0o644))
}

// This is the collision the variant exists to prevent: upstream v0.2.2 and v0.5.1 both ship
// anon_nullifier_kyc_transfer with the same name and different proving keys. Resolving without the variant would
// hand a V1 pool the V0 key, and the proof would be rejected on chain by the V1 verifier contract.
func TestLoadCircuitReadsVariantDir(t *testing.T) {
	root := t.TempDir()
	const circuit = "anon_nullifier_kyc_transfer"
	writeArtifacts(t, root, "V0", circuit, "v0")
	config := &zetosignerapi.SnarkProverConfig{CircuitsDir: root, ProvingKeysDir: root}

	// the wasm bytes are not a real circuit, so the calculator construction fails — but the artifact reads that precede
	// it are what this asserts, so the error must come from the witness calculator, not a missing file
	_, _, err := loadCircuit(t.Context(), "V0", circuit, config)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "no such file")

	_, _, err = loadCircuit(t.Context(), "V1", circuit, config)
	assert.Regexp(t, "V1/.*no such file", err)
}

// A flat root (no V<n> directories) is read by a prover whose artifacts are not laid out per variant, so a node whose
// artifact volume predates this layout keeps working.
func TestLoadCircuitFlatLayout(t *testing.T) {
	root := t.TempDir()
	const circuit = "anon"
	writeArtifacts(t, root, "", circuit, "flat")
	config := &zetosignerapi.SnarkProverConfig{CircuitsDir: root, ProvingKeysDir: root}

	_, _, err := loadCircuit(t.Context(), "", circuit, config)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "no such file")

	_, _, err = loadCircuit(t.Context(), "V0", circuit, config)
	assert.Regexp(t, "no such file", err)
}

func TestVariantDirNameMatchesTypesPackage(t *testing.T) {
	// kept in step with types.ZetoVariant.ArtifactDirName(); duplicated to keep the signer dependency-free
	assert.Equal(t, "V0", variantDirName(0))
	assert.Equal(t, "V1", variantDirName(1))
}

func newVariantTestProver(t *testing.T, artifactsPerVariant bool, loader func(variantDir string) ([]byte, error)) *snarkProver {
	// One worker per circuit, so requests for the same circuit land on the same worker index. Without that the calls
	// would get different worker slots and miss the cache regardless of the key, and the test would pass either way.
	prover, err := newSnarkProver(&zetosignerapi.SnarkProverConfig{
		CircuitsDir:         "test",
		ProvingKeysDir:      "test",
		MaxProverPerCircuit: confutil.P(1),
	}, artifactsPerVariant)
	require.NoError(t, err)
	prover.circuitLoader = func(ctx context.Context, variantDir string, circuitID string, config *zetosignerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		key, err := loader(variantDir)
		if err != nil {
			return nil, nil, err
		}
		return &testWitnessCalculator{}, key, nil
	}
	prover.proofGenerator = func(ctx context.Context, wtns []byte, provingKey []byte) (*types.ZKProof, error) {
		return &types.ZKProof{
			Proof: &types.ProofData{
				A: []string{"a"},
				B: [][]string{{"b1.1", "b1.2"}, {"b2.1", "b2.2"}},
				C: []string{"c"},
			},
		}, nil
	}
	return prover
}

func signForVariant(t *testing.T, prover *snarkProver, variant uint64) error {
	alice := common.NewTestKeypair()
	bob := common.NewTestKeypair()
	inputValues := []*big.Int{big.NewInt(30), big.NewInt(40)}
	salt1 := crypto.NewSalt()
	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
	salt2 := crypto.NewSalt()
	input2, _ := poseidon.Hash([]*big.Int{inputValues[1], salt2, alice.PublicKey.X, alice.PublicKey.Y})
	tokenSecrets, err := json.Marshal(&pb.TokenSecrets_Fungible{
		InputValues:  []uint64{30, 40},
		OutputValues: []uint64{32, 38},
	})
	require.NoError(t, err)

	req := pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Name:    "anon_nullifier_kyc_transfer",
			Type:    string(zetosignerapi.Transfer),
			Variant: variant,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{input1.Text(16), input2.Text(16)},
			InputSalts:       []string{salt1.Text(16), salt2.Text(16)},
			InputOwner:       "alice/key0",
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners: []string{
				common.EncodeBabyJubJubPublicKey(bob.PublicKey),
				common.EncodeBabyJubJubPublicKey(alice.PublicKey),
			},
			TokenSecrets: tokenSecrets,
			TokenType:    pb.TokenType_fungible,
		},
	}
	payload, err := proto.Marshal(&req)
	require.NoError(t, err)
	_, err = prover.Sign(t.Context(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"),
		zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
	return err
}

// The prover caches witness calculators and proving keys per worker. That key must include the variant, or a second
// pool on a different variant would be served the first pool's cached artifacts for the same circuit name — which is
// exactly the anon_nullifier_kyc_transfer collision between upstream v0.2.2 and v0.5.1.
func TestProverLoadsArtifactsPerVariant(t *testing.T) {
	for _, tc := range []struct {
		artifactsPerVariant bool
		expectedDirs        []string
	}{
		{artifactsPerVariant: true, expectedDirs: []string{"V0", "V1"}},
		{artifactsPerVariant: false, expectedDirs: []string{"", ""}},
	} {
		var mu sync.Mutex
		var loaded []string
		prover := newVariantTestProver(t, tc.artifactsPerVariant, func(variantDir string) ([]byte, error) {
			mu.Lock()
			defer mu.Unlock()
			loaded = append(loaded, variantDir)
			return []byte("key-" + variantDir), nil
		})

		// the same circuit name, requested for each variant in turn, then again to hit the cache
		for _, variant := range []uint64{0, 1, 0, 1} {
			require.NoError(t, signForVariant(t, prover, variant), "variant %d", variant)
		}
		// Both variants had to load their own artifacts. A cache keyed on circuit name alone would show a single load,
		// meaning the V1 request was served the V0 proving key.
		assert.Equal(t, tc.expectedDirs, loaded)
	}
}

func TestProverLoadFailure(t *testing.T) {
	prover := newVariantTestProver(t, true, func(variantDir string) ([]byte, error) {
		return nil, fmt.Errorf("pop")
	})
	err := signForVariant(t, prover, 1)
	assert.Regexp(t, "PD210163.*anon_nullifier_kyc_transfer.*V1.*pop", err)

	// The failure must not leave the prover locked
	err = signForVariant(t, prover, 1)
	assert.Regexp(t, "PD210163", err)
}
