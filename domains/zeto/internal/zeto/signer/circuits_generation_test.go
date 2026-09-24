/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

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

// writeArtifacts lays down a circuit's .wasm and .zkey under root, optionally inside a generation directory.
func writeArtifacts(t *testing.T, root, genDir, circuit, marker string) {
	t.Helper()
	base := root
	if genDir != "" {
		base = path.Join(root, genDir)
	}
	require.NoError(t, os.MkdirAll(path.Join(base, circuit+"_js"), 0o755))
	require.NoError(t, os.WriteFile(path.Join(base, circuit+"_js", circuit+".wasm"), []byte("wasm-"+marker), 0o644))
	require.NoError(t, os.WriteFile(path.Join(base, circuit+".zkey"), []byte("zkey-"+marker), 0o644))
}

// This is the collision the generation axis exists to prevent: upstream v0.2.2 and v0.5.1 both ship
// anon_nullifier_kyc_transfer with the same name and different proving keys. Resolving without the generation would
// hand a V1 pool the V0 key, and the proof would be rejected on chain by the V1 verifier contract.
func TestResolveArtifactSeparatesCollidingCircuitNames(t *testing.T) {
	root := t.TempDir()
	const circuit = "anon_nullifier_kyc_transfer"
	writeArtifacts(t, root, "V0", circuit, "v0")
	writeArtifacts(t, root, "V1", circuit, "v1")

	zkeyV0, err := os.ReadFile(resolveArtifact(root, 0, circuit+".zkey"))
	require.NoError(t, err)
	zkeyV1, err := os.ReadFile(resolveArtifact(root, 1, circuit+".zkey"))
	require.NoError(t, err)

	assert.Equal(t, "zkey-v0", string(zkeyV0))
	assert.Equal(t, "zkey-v1", string(zkeyV1))
	assert.NotEqual(t, string(zkeyV0), string(zkeyV1))
}

// A flat root (no V<n> directories) still resolves, so a node whose artifact volume predates this layout keeps working.
func TestResolveArtifactFallsBackToFlatLayout(t *testing.T) {
	root := t.TempDir()
	const circuit = "anon"
	writeArtifacts(t, root, "", circuit, "flat")

	for _, generation := range []uint64{0, 1} {
		zkey, err := os.ReadFile(resolveArtifact(root, generation, circuit+".zkey"))
		require.NoError(t, err, "generation %d", generation)
		assert.Equal(t, "zkey-flat", string(zkey))
	}
}

// A generation directory wins over a flat file of the same name, so a partially migrated volume prefers the correct one.
func TestResolveArtifactPrefersGenerationDirOverFlat(t *testing.T) {
	root := t.TempDir()
	const circuit = "deposit"
	writeArtifacts(t, root, "", circuit, "flat")
	writeArtifacts(t, root, "V1", circuit, "v1")

	v1, err := os.ReadFile(resolveArtifact(root, 1, circuit+".zkey"))
	require.NoError(t, err)
	assert.Equal(t, "zkey-v1", string(v1))

	// V0 has no directory of its own, so it still finds the flat copy
	v0, err := os.ReadFile(resolveArtifact(root, 0, circuit+".zkey"))
	require.NoError(t, err)
	assert.Equal(t, "zkey-flat", string(v0))
}

func TestLoadCircuitReadsGenerationScopedArtifacts(t *testing.T) {
	root := t.TempDir()
	const circuit = "anon_nullifier_kyc_transfer"
	writeArtifacts(t, root, "V0", circuit, "v0")
	writeArtifacts(t, root, "V1", circuit, "v1")

	config := &zetosignerapi.SnarkProverConfig{CircuitsDir: root, ProvingKeysDir: root}

	// the wasm bytes are not a real circuit, so the calculator construction fails — but the proving key read that
	// precedes it is what this asserts, and the error must come from the witness calculator, not a missing file
	_, zkey, err := loadCircuit(context.Background(), 1, circuit, config)
	if err == nil {
		assert.Equal(t, "zkey-v1", string(zkey))
	} else {
		assert.NotContains(t, err.Error(), "no such file")
	}
}

func TestLoadCircuitRequiresConfiguredDirs(t *testing.T) {
	ctx := context.Background()
	_, _, err := loadCircuit(ctx, 0, "anon", &zetosignerapi.SnarkProverConfig{})
	require.Error(t, err)
	assert.Regexp(t, "PD210000|PD2100", err)

	_, _, err = loadCircuit(ctx, 0, "anon", &zetosignerapi.SnarkProverConfig{CircuitsDir: "somewhere"})
	require.Error(t, err)
}

func TestGenerationDirNameMatchesTypesPackage(t *testing.T) {
	// kept in step with types.ZetoReleaseGeneration.ArtifactDirName(); duplicated to keep the signer dependency-free
	assert.Equal(t, "V0", generationDirName(0))
	assert.Equal(t, "V1", generationDirName(1))
}

// The prover caches witness calculators and proving keys per worker. That key must include the generation, or a
// second pool on a different generation would be served the first pool's cached artifacts for the same circuit name —
// which is exactly the anon_nullifier_kyc_transfer collision between upstream v0.2.2 and v0.5.1.
func TestProverCachesArtifactsPerGeneration(t *testing.T) {
	// One worker per circuit, so both requests land on the same worker index. Without that the two calls would get
	// different worker slots and miss the cache regardless of the key, and the test would pass either way.
	prover, err := newSnarkProver(&zetosignerapi.SnarkProverConfig{
		CircuitsDir:         "test",
		ProvingKeysDir:      "test",
		MaxProverPerCircuit: confutil.P(1),
	})
	require.NoError(t, err)

	var mu sync.Mutex
	var loaded []uint64
	prover.circuitLoader = func(ctx context.Context, generation uint64, circuitID string, config *zetosignerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		mu.Lock()
		defer mu.Unlock()
		loaded = append(loaded, generation)
		return &testWitnessCalculator{}, []byte(fmt.Sprintf("key-V%d", generation)), nil
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

	// the same circuit name, requested for each generation in turn
	for _, generation := range []uint64{0, 1} {
		req := pb.ProvingRequest{
			Circuit: &pb.Circuit{
				Name:       "anon_nullifier_kyc_transfer",
				Type:       string(zetosignerapi.Transfer),
				Generation: generation,
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
		_, err = prover.Sign(context.Background(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"),
			zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
		require.NoError(t, err, "generation %d", generation)
	}

	// Both generations had to load their own artifacts. A cache keyed on circuit name alone would show a single load,
	// meaning the V1 request was served the V0 proving key.
	assert.ElementsMatch(t, []uint64{0, 1}, loaded)
}
