/*
 * Copyright © 2025 Kaleido, Inc.
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

package smt

import (
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
)

func TestMerkleTreeRootHash(t *testing.T) {
	root := &MerkleTreeRoot{
		SmtName:   "test-merkle-tree",
		RootIndex: pldtypes.Bytes32{0x01, 0x02, 0x03},
	}

	hash, err := root.Hash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Equal(t, 64, len(hash)) // SHA256 hex string is 64 chars

	// Same inputs should produce same hash
	hash2, err := root.Hash()
	assert.NoError(t, err)
	assert.Equal(t, hash, hash2)

	// Different root index should produce different hash
	root2 := &MerkleTreeRoot{
		SmtName:   "test-merkle-tree",
		RootIndex: pldtypes.Bytes32{0x04, 0x05, 0x06},
	}
	hash3, err := root2.Hash()
	assert.NoError(t, err)
	assert.NotEqual(t, hash, hash3)

	// Different SMT name should produce different hash
	root3 := &MerkleTreeRoot{
		SmtName:   "different-merkle-tree",
		RootIndex: pldtypes.Bytes32{0x01, 0x02, 0x03},
	}
	hash4, err := root3.Hash()
	assert.NoError(t, err)
	assert.NotEqual(t, hash, hash4)
}

func TestMerkleTreeRootHash_EIP712(t *testing.T) {
	root := &MerkleTreeRoot{
		SmtName:   "test-merkle-tree",
		RootIndex: pldtypes.Bytes32{0x01, 0x02, 0x03},
	}

	hash, err := root.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Equal(t, 64, len(hash)) // EIP712 hash is 64 hex chars

	// Same inputs should produce same hash
	hash2, err := root.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, hash, hash2)

	// Different root should produce different hash
	root2 := &MerkleTreeRoot{
		SmtName:   "test-merkle-tree",
		RootIndex: pldtypes.Bytes32{0x04, 0x05, 0x06},
	}
	hash3, err := root2.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEqual(t, hash, hash3)
}
func TestMerkleTreeRootHashWithVariousValues(t *testing.T) {
	// Test with empty root index
	root := &MerkleTreeRoot{
		SmtName:   "empty-root",
		RootIndex: pldtypes.Bytes32{},
	}

	hash, err := root.Hash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Test with max root index
	root2 := &MerkleTreeRoot{
		SmtName:   "max-root",
		RootIndex: pldtypes.Bytes32{0xFF, 0xFF, 0xFF, 0xFF},
	}

	hash2, err := root2.Hash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash2)
	assert.NotEqual(t, hash, hash2)
}

func TestMerkleTreeRootHash_EIP712WithDifferentFields(t *testing.T) {
	baseRoot := &MerkleTreeRoot{
		SmtName:   "test-merkle-tree",
		RootIndex: pldtypes.Bytes32{0x01, 0x02, 0x03},
	}

	hash1, err := baseRoot.Hash_EIP712(t.Context())
	assert.NoError(t, err)

	// Change SmtName
	root2 := &MerkleTreeRoot{
		SmtName:   "different-name",
		RootIndex: pldtypes.Bytes32{0x01, 0x02, 0x03},
	}
	hash2, err := root2.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash2)

	// Change RootIndex
	root3 := &MerkleTreeRoot{
		SmtName:   "test-merkle-tree",
		RootIndex: pldtypes.Bytes32{0xFF, 0xFF, 0xFF},
	}
	hash3, err := root3.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)
}

func TestMerkleTreeRootHash_EIP712Error1(t *testing.T) {
	oldAbi := MerkleTreeRootABI
	defer func() { MerkleTreeRootABI = oldAbi }()

	MerkleTreeRootABI = &abi.Parameter{
		Name: "MerkleTreeRoot",
		Type: "bad-type",
	}
	root := &MerkleTreeRoot{}
	_, err := root.Hash_EIP712(t.Context())
	assert.EqualError(t, err, "FF22025: Unsupported elementary type 'bad' in ABI type 'bad-type'")
}
func TestMerkleTreeRootHash_EIP712Error2(t *testing.T) {
	oldAbi := MerkleTreeRootABI
	defer func() { MerkleTreeRootABI = oldAbi }()

	MerkleTreeRootABI = &abi.Parameter{
		Name: "MerkleTreeRoot",
		Type: "uint256",
	}
	root := &MerkleTreeRoot{}
	_, err := root.Hash_EIP712(t.Context())
	assert.EqualError(t, err, "FF22074: Type primary type must be a struct/tuple: uint256")
}

func TestMerkleTreeRootHash_EIP712Error3(t *testing.T) {
	oldAbi := MerkleTreeRootABI
	defer func() { MerkleTreeRootABI = oldAbi }()

	MerkleTreeRootABI = MerkleTreeNodeABI
	root := &MerkleTreeRoot{}
	_, err := root.Hash_EIP712(t.Context())
	assert.Error(t, err)
}
