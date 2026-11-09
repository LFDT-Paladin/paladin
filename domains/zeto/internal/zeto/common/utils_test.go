package common

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	corepb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/iden3/go-iden3-crypto/babyjub"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/constants"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	pb "github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/smt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IsNullifiersToken
func TestIsNullifiersToken(t *testing.T) {
	assert.True(t, IsNullifiersToken(constants.TOKEN_ANON_NULLIFIER))
	assert.True(t, IsNullifiersToken(constants.TOKEN_NF_ANON_NULLIFIER))
	assert.False(t, IsNullifiersToken("other"))
}

func TestCoinStateIDs(t *testing.T) {
	ctx := t.Context()
	hexID := "0x0d7b11e7bb9f808761aba8e35b8c57839d8c17b2479f7a89b88f5dfa58d0df13"

	// The state store serializes uint256 as base-10
	var commitments []pldtypes.HexUint256
	err := json.Unmarshal([]byte(`["6097512797744793758821124728700260884882015502756931181589044059211954183955","0x1234"]`), &commitments)
	require.NoError(t, err)
	ids, err := CoinStateIDs(ctx, commitments)
	require.NoError(t, err)
	assert.Equal(t, []pldtypes.Bytes32{
		pldtypes.MustParseBytes32(hexID),
		pldtypes.MustParseBytes32("0x0000000000000000000000000000000000000000000000000000000000001234"),
	}, ids)

	tooLarge := new(big.Int).Exp(big.NewInt(2), big.NewInt(257), nil)
	_, err = CoinStateIDs(ctx, []pldtypes.HexUint256{pldtypes.HexUint256(*tooLarge)})
	assert.Regexp(t, "PD020027", err)
}

// EncodeProof: Create a dummy SnarkProof and verify the output.
func TestEncodeProof(t *testing.T) {
	// Create a dummy proof using the real corepb types.
	proof := &corepb.SnarkProof{
		A: []string{"a1", "a2"},
		B: []*corepb.B_Item{
			{Items: []string{"b00", "b01"}},
			{Items: []string{"b10", "b11"}},
		},
		C: []string{"c1", "c2"},
	}
	result := EncodeProof(proof)
	expected := map[string]interface{}{
		"pA": []string{"a1", "a2"},
		"pB": [][]string{
			{"b01", "b00"},
			{"b11", "b10"},
		},
		"pC": []string{"c1", "c2"},
	}
	assert.Equal(t, expected, result)
}

// TestEncodeTransactionData uses a table-driven approach to test both valid and invalid scenarios.
func TestEncodeTransactionData(t *testing.T) {
	tests := map[string]struct {
		transactionId string
		expected      string
		expectError   bool
	}{
		"valid": {
			transactionId: "0x8572b34655888710ea27adbda526c5789576b7072cb0494dab3c8c7891549934",
			expected:      "0x000100008572b34655888710ea27adbda526c5789576b7072cb0494dab3c8c789154993400000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000000",
			expectError:   false,
		},
		"invalid TransactionId": {
			transactionId: "invalid",
			expected:      "",
			expectError:   true,
		},
	}

	ctx := context.Background()
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			txn := &prototk.TransactionSpecification{
				TransactionId: tc.transactionId,
			}
			result, err := EncodeTransactionData(ctx, txn, nil)
			if tc.expectError {
				assert.Error(t, err, "expected an error when transactionId is invalid")
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err, "expected no error for valid transactionId")
				assert.Equal(t, tc.expected, result.String(), "result should equal the expected concatenation")
			}
		})
	}
}

func TestEncodeTransactionData_InvalidTransactionId(t *testing.T) {
	ctx := context.Background()
	txn := &prototk.TransactionSpecification{
		TransactionId: "invalid",
	}
	result, err := EncodeTransactionData(ctx, txn, nil)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestEncodeTransactionData_InvalidInfoStateId(t *testing.T) {
	ctx := context.Background()
	txn := &prototk.TransactionSpecification{
		TransactionId: "0x8572b34655888710ea27adbda526c5789576b7072cb0494dab3c8c7891549934",
	}
	infoStates := []*prototk.EndorsableState{
		{Id: "invalid"},
	}
	result, err := EncodeTransactionData(ctx, txn, infoStates)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestLoadBabyJubKey(t *testing.T) {
	var validComp babyjub.PublicKeyComp
	for i := range validComp {
		// Use a non-zero value (here simply i+1) for each byte.
		validComp[i] = byte(i + 1)
	}
	// Marshal the PublicKeyComp to text to obtain the payload.
	validPayload, err := validComp.MarshalText()
	require.NoError(t, err, "failed to marshal valid PublicKeyComp")

	tests := map[string]struct {
		payload     []byte
		expectError bool
	}{
		"invalid hex": {
			payload:     []byte("zzz"),
			expectError: true,
		},
		"success": {
			payload:     validPayload,
			expectError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pk, err := LoadBabyJubKey(tc.payload)
			if tc.expectError {
				assert.Error(t, err, "expected an error for payload %q", tc.payload)
				assert.Nil(t, pk, "expected public key to be nil on error")
			} else {
				assert.NoError(t, err, "unexpected error for payload %q", tc.payload)
				require.NotNil(t, pk, "expected a valid public key")
				// Re-compress the returned public key.
				reCompressed := pk.Compress()
				assert.Equal(t, validComp, reCompressed, "re-compressed key should equal original")
			}
		})
	}
}
func TestCryptoRandBN254(t *testing.T) {
	fieldModulus, ok := new(big.Int).SetString(modulus, 10)
	require.True(t, ok, "failed to parse field modulus")

	// Run multiple iterations to test the randomness and verify the range.
	const iterations = 10
	for i := 0; i < iterations; i++ {
		tokenValue, err := CryptoRandBN254()
		assert.NoError(t, err, "CryptoRandBN254 returned an error on iteration %d", i)
		require.NotNil(t, tokenValue, "CryptoRandBN254 returned a nil token on iteration %d", i)

		// Ensure the generated token is in the range [0, fieldModulus).
		// tokenValue must be less than fieldModulus.
		assert.Less(t, tokenValue.Cmp(fieldModulus), 0, "token value %s is not less than field modulus %s on iteration %d",
			tokenValue.String(), fieldModulus.String(), i)
	}
}

func TestNewMerkleTreeSpec(t *testing.T) {
	ctx := context.Background()
	testCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func(ctx context.Context, req *pb.FindAvailableStatesRequest) (*pb.FindAvailableStatesResponse, error) {
			return &pb.FindAvailableStatesResponse{}, nil
		},
	}

	spec, err := NewMerkleTreeSpec(ctx, "testSmt", smt.StatesTree, testCallbacks, "smt_root_schema", "smt_node_schema", "test_query_context")
	require.NoError(t, err)
	assert.Equal(t, smt.StatesTree, spec.Type)

	spec, err = NewMerkleTreeSpec(ctx, "testSmt", smt.LockedStatesTree, testCallbacks, "smt_root_schema", "smt_node_schema", "test_query_context")
	require.NoError(t, err)
	assert.Equal(t, smt.LockedStatesTree, spec.Type)

	spec, err = NewMerkleTreeSpec(ctx, "testSmt", smt.KycStatesTree, testCallbacks, "smt_root_schema", "smt_node_schema", "test_query_context")
	require.NoError(t, err)
	assert.Equal(t, smt.KycStatesTree, spec.Type)

	spec, err = NewMerkleTreeSpec(ctx, "testSmt", 3, testCallbacks, "smt_root_schema", "smt_node_schema", "test_query_context")
	require.ErrorContains(t, err, "PD210140: Unknown states merkle tree type: 3")
}
