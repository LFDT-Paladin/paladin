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

func TestMerkleTreeNodeHash(t *testing.T) {
	node := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0x01},
		Index:      pldtypes.Bytes32{0x02},
		Type:       pldtypes.HexBytes{0x03},
		LeftChild:  pldtypes.Bytes32{0x04},
		RightChild: pldtypes.Bytes32{0x05},
	}

	hash, err := node.Hash("test-smt")
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Equal(t, 64, len(hash)) // SHA256 hex string is 64 chars

	// Same inputs should produce same hash
	hash2, err := node.Hash("test-smt")
	assert.NoError(t, err)
	assert.Equal(t, hash, hash2)

	// Different SMT names should produce different hashes
	hash3, err := node.Hash("different-smt")
	assert.NoError(t, err)
	assert.NotEqual(t, hash, hash3)

	// Different nodes should produce different hashes
	node2 := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0x01},
		Index:      pldtypes.Bytes32{0x02},
		Type:       pldtypes.HexBytes{0x03},
		LeftChild:  pldtypes.Bytes32{0x04},
		RightChild: pldtypes.Bytes32{0x06}, // Different right child
	}
	hash4, err := node2.Hash("test-smt")
	assert.NoError(t, err)
	assert.NotEqual(t, hash, hash4)
}

func TestMerkleTreeNodeHash_EIP712(t *testing.T) {
	node := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0x01},
		Index:      pldtypes.Bytes32{0x02},
		Type:       pldtypes.HexBytes{0x03},
		LeftChild:  pldtypes.Bytes32{0x04},
		RightChild: pldtypes.Bytes32{0x05},
	}

	hash, err := node.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Equal(t, 64, len(hash)) // EIP712 hash is also 64 hex chars

	// Same inputs should produce same hash
	hash2, err := node.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, hash, hash2)

	// Different nodes should produce different hashes
	node2 := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0x06},
		Index:      pldtypes.Bytes32{0x02},
		Type:       pldtypes.HexBytes{0x03},
		LeftChild:  pldtypes.Bytes32{0x04},
		RightChild: pldtypes.Bytes32{0x05},
	}
	hash3, err := node2.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEqual(t, hash, hash3)
}
func TestMerkleTreeNodeHashWithVariousTypes(t *testing.T) {
	// Test with different node types
	node := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{},
		Index:      pldtypes.Bytes32{},
		Type:       pldtypes.HexBytes{},
		LeftChild:  pldtypes.Bytes32{},
		RightChild: pldtypes.Bytes32{},
	}

	hash, err := node.Hash("empty-node")
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Test with all bytes set to max
	node2 := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0xFF, 0xFF, 0xFF, 0xFF},
		Index:      pldtypes.Bytes32{0xFF, 0xFF, 0xFF, 0xFF},
		Type:       pldtypes.HexBytes{0xFF},
		LeftChild:  pldtypes.Bytes32{0xFF, 0xFF, 0xFF, 0xFF},
		RightChild: pldtypes.Bytes32{0xFF, 0xFF, 0xFF, 0xFF},
	}

	hash2, err := node2.Hash("max-node")
	assert.NoError(t, err)
	assert.NotEmpty(t, hash2)
	assert.NotEqual(t, hash, hash2)
}

func TestMerkleTreeNodeHash_EIP712WithDifferentFields(t *testing.T) {
	baseNode := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0x01},
		Index:      pldtypes.Bytes32{0x02},
		Type:       pldtypes.HexBytes{0x03},
		LeftChild:  pldtypes.Bytes32{0x04},
		RightChild: pldtypes.Bytes32{0x05},
	}

	hash1, err := baseNode.Hash_EIP712(t.Context())
	assert.NoError(t, err)

	// Change RefKey
	node2 := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0xFF},
		Index:      pldtypes.Bytes32{0x02},
		Type:       pldtypes.HexBytes{0x03},
		LeftChild:  pldtypes.Bytes32{0x04},
		RightChild: pldtypes.Bytes32{0x05},
	}
	hash2, err := node2.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash2)

	// Change Index
	node3 := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0x01},
		Index:      pldtypes.Bytes32{0xFF},
		Type:       pldtypes.HexBytes{0x03},
		LeftChild:  pldtypes.Bytes32{0x04},
		RightChild: pldtypes.Bytes32{0x05},
	}
	hash3, err := node3.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)

	// Change LeftChild
	node4 := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0x01},
		Index:      pldtypes.Bytes32{0x02},
		Type:       pldtypes.HexBytes{0x03},
		LeftChild:  pldtypes.Bytes32{0xFF},
		RightChild: pldtypes.Bytes32{0x05},
	}
	hash4, err := node4.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash4)

	// Change RightChild
	node5 := &MerkleTreeNode{
		RefKey:     pldtypes.Bytes32{0x01},
		Index:      pldtypes.Bytes32{0x02},
		Type:       pldtypes.HexBytes{0x03},
		LeftChild:  pldtypes.Bytes32{0x04},
		RightChild: pldtypes.Bytes32{0xFF},
	}
	hash5, err := node5.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEqual(t, hash1, hash5)
}

func TestMerkleTreeNodeHash_EIP712Error1(t *testing.T) {
	oldAbi := MerkleTreeNodeABI
	defer func() { MerkleTreeNodeABI = oldAbi }()

	MerkleTreeNodeABI = &abi.Parameter{
		Name: "MerkleTreeNode",
		Type: "bad-type",
	}
	node := &MerkleTreeNode{}
	_, err := node.Hash_EIP712(t.Context())
	assert.EqualError(t, err, "FF22025: Unsupported elementary type 'bad' in ABI type 'bad-type'")
}
func TestMerkleTreeNodeHash_EIP712Error2(t *testing.T) {
	oldAbi := MerkleTreeNodeABI
	defer func() { MerkleTreeNodeABI = oldAbi }()

	MerkleTreeNodeABI = &abi.Parameter{
		Name: "MerkleTreeNode",
		Type: "uint256",
	}
	node := &MerkleTreeNode{}
	_, err := node.Hash_EIP712(t.Context())
	assert.EqualError(t, err, "FF22074: Type primary type must be a struct/tuple: uint256")
}

func TestMerkleTreeNodeHash_EIP712Error3(t *testing.T) {
	oldAbi := MerkleTreeNodeABI
	defer func() { MerkleTreeNodeABI = oldAbi }()

	MerkleTreeNodeABI = MerkleTreeRootABI
	node := &MerkleTreeNode{}
	_, err := node.Hash_EIP712(t.Context())
	assert.Error(t, err)
}
