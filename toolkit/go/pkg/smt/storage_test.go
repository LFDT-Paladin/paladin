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

package smt

import (
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/core"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/node"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/smt"
	"github.com/LFDT-Paladin/smt/pkg/utxo"
	utxocore "github.com/LFDT-Paladin/smt/pkg/utxo/core"
	"github.com/stretchr/testify/assert"
)

func returnCustomError() (*prototk.FindAvailableStatesResponse, error) {
	return nil, errors.New("test error")
}

func returnEmptyStates() (*prototk.FindAvailableStatesResponse, error) {
	return &prototk.FindAvailableStatesResponse{}, nil
}

func returnBadData() (*prototk.FindAvailableStatesResponse, error) {
	return &prototk.FindAvailableStatesResponse{
		States: []*prototk.StoredState{
			{
				DataJson: "bad data",
			},
		},
	}, nil
}

func returnNode(t int) func() (*prototk.FindAvailableStatesResponse, error) {
	var data []byte
	if t == 0 {
		data, _ = json.Marshal(map[string]string{"rootIndex": "0x1234567890123456789012345678901234567890123456789012345678901234"})
	} else if t == 1 {
		data, _ = json.Marshal(map[string]string{
			"index":      "0x197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521",
			"leftChild":  "0x0000000000000000000000000000000000000000000000000000000000000000",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"type":       "0x02", // leaf node
		})
	} else if t == 2 {
		data, _ = json.Marshal(map[string]string{
			"leftChild":  "0x197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0xd23ae67af3b0e9f4854eb76954c27c7607b2a37b633d6b107e607cee460a6425",
			"type":       "0x01", // branch node
		})
	} else if t == 3 {
		data, _ = json.Marshal(map[string]string{
			"leftChild":  "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0xd23ae67af3b0e9f4854eb76954c27c7607b2a37b633d6b107e607cee460a6425",
			"type":       "0x01", // branch node
		})
	} else if t == 4 {
		data, _ = json.Marshal(map[string]string{
			"leftChild":  "0x197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"type":       "0x01", // branch node
		})
	} else if t == 5 {
		data, _ = json.Marshal(map[string]string{
			"index":      "baddata",
			"leftChild":  "0x0000000000000000000000000000000000000000000000000000000000000000",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"type":       "0x02", // leaf node
		})
	} else if t == 6 {
		data, _ = json.Marshal(map[string]string{
			"index":      "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"leftChild":  "0x0000000000000000000000000000000000000000000000000000000000000000",
			"refKey":     "0x040a1f5b3aca49a82b256b9250a0665e8e6fee7713d7d67fbf0d9e4728561fe8",
			"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"type":       "0x02", // leaf node
		})
	}
	return func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					DataJson: string(data),
				},
			},
		}, nil
	}
}

func TestStorage(t *testing.T) {
	stateQueryConext := pldtypes.ShortID()
	// for Zeto, use Poseidon hasher and not EIP712 hashing
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnCustomError}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	mt, err := smt.NewMerkleTree(t.Context(), storage, 64)
	assert.EqualError(t, err, "test error")
	assert.NotNil(t, storage)
	assert.Nil(t, mt)

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	mt, err = smt.NewMerkleTree(t.Context(), storage, 64)
	assert.NoError(t, err)
	assert.NotNil(t, storage)
	assert.NotNil(t, mt)
	assert.Nil(t, storage.(*statesStorage).rootNode)
	assert.Equal(t, 0, len(storage.(*statesStorage).committedNewNodes))
	newStates, err := storage.(*statesStorage).GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Len(t, newStates, 0)
	idx, err := storage.(*statesStorage).GetRootNodeRef(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", idx.Hex())

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnBadData}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	mt, err = smt.NewMerkleTree(t.Context(), storage, 64)
	assert.EqualError(t, err, "PD021203: Failed to unmarshal root node index. invalid character 'b' looking for beginning of value")
	assert.NotNil(t, storage)
	assert.Nil(t, mt)

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(0)}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	mt, err = smt.NewMerkleTree(t.Context(), storage, 64)
	assert.NoError(t, err)
	assert.NotNil(t, storage)
	assert.NotNil(t, mt)
	assert.Nil(t, storage.(*statesStorage).pendingNodesTx)

	newStates, err = storage.(*statesStorage).GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Empty(t, newStates)
	idx, err = storage.(*statesStorage).GetRootNodeRef(t.Context())
	assert.NoError(t, err)
	assert.NotEmpty(t, idx)

	// test rollback
	tx, err := storage.BeginTx(t.Context())
	assert.NoError(t, err)
	idx1, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	err = tx.UpsertRootNodeRef(t.Context(), idx1)
	assert.NoError(t, err)
	assert.Equal(t, "d204000000000000000000000000000000000000000000000000000000000000", storage.(*statesStorage).pendingNodesTx.inflightRoot.Hex())
	assert.Nil(t, tx.Rollback(t.Context()))
	newStates, err = storage.(*statesStorage).GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(newStates))
}

func TestUpsertRootNodeIndex(t *testing.T) {
	stateQueryConext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	_, _ = smt.NewMerkleTree(t.Context(), storage, 64)
	assert.NotNil(t, storage)
	tx, err := storage.BeginTx(t.Context())
	assert.NoError(t, err)
	idx1, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	err = tx.UpsertRootNodeRef(t.Context(), idx1)
	assert.NoError(t, err)
	assert.Equal(t, "d204000000000000000000000000000000000000000000000000000000000000", storage.(*statesStorage).pendingNodesTx.inflightRoot.Hex())
	assert.Nil(t, tx.Commit(t.Context()))
	newStates, err := storage.(*statesStorage).GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(newStates))

	idx2, err := storage.(*statesStorage).GetRootNodeRef(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, idx1, idx2)
}

func TestGetNode(t *testing.T) {
	stateQueryConext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnCustomError}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	_, err := storage.GetNode(t.Context(), idx)
	assert.EqualError(t, err, "test error")

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	_, err = storage.GetNode(t.Context(), idx)
	assert.EqualError(t, err, core.ErrNotFound.Error())

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(1)}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	n, err := storage.GetNode(t.Context(), idx)
	assert.NoError(t, err)
	assert.NotNil(t, n)
	assert.Equal(t, "197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521", n.Index().Hex())
	assert.Equal(t, core.NodeTypeLeaf, n.Type())

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(2)}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	n, err = storage.GetNode(t.Context(), idx)
	assert.NoError(t, err)
	assert.NotNil(t, n)
	assert.Empty(t, n.Index())
	assert.Equal(t, "197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521", n.LeftChild().Hex())

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(3)}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	_, err = storage.GetNode(t.Context(), idx)
	assert.EqualError(t, err, "inputs values not inside Finite Field")

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(4)}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	_, err = storage.GetNode(t.Context(), idx)
	assert.EqualError(t, err, "inputs values not inside Finite Field")

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(5)}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	_, err = storage.GetNode(t.Context(), idx)
	assert.ErrorContains(t, err, "PD021204: Failed to unmarshal Merkle Tree Node from state json. PD020007: Invalid hex")

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(6)}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	_, err = storage.GetNode(t.Context(), idx)
	assert.ErrorContains(t, err, "PD021204: Failed to unmarshal Merkle Tree Node from state json. PD020008: Failed to parse value as 32 byte hex string")

	// test with committed nodes
	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	tx1, err := storage.BeginTx(t.Context())
	assert.NoError(t, err)
	n1, _ := node.NewLeafNode(node.NewIndexOnly(idx), nil)
	err = tx1.InsertNode(t.Context(), n1)
	assert.NoError(t, err)
	assert.Nil(t, tx1.Commit(t.Context()))
	n2, err := storage.GetNode(t.Context(), n1.Ref())
	assert.NoError(t, err)
	assert.Equal(t, n1, n2)

	// test with pending nodes (called when we are still updating a leaf node path up to the root)
	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	tx2, err := storage.BeginTx(t.Context())
	assert.NoError(t, err)
	n3, _ := node.NewLeafNode(node.NewIndexOnly(idx), nil)
	err = tx2.InsertNode(t.Context(), n3)
	assert.NoError(t, err)
	n4, err := storage.GetNode(t.Context(), n3.Ref())
	assert.NoError(t, err)
	assert.Equal(t, n3, n4)
}

func TestInsertNode(t *testing.T) {
	stateQueryConext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	assert.NotNil(t, storage)
	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	n, _ := node.NewLeafNode(node.NewIndexOnly(idx), nil)

	tx1, err := storage.BeginTx(t.Context())
	assert.NoError(t, err)
	err = tx1.InsertNode(t.Context(), n)
	assert.NoError(t, err)
	err = tx1.UpsertRootNodeRef(t.Context(), n.Ref())
	assert.NoError(t, err)
	newStates, err := storage.(*statesStorage).GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(newStates))
	assert.Nil(t, tx1.Commit(t.Context()))
	newStates, err = storage.(*statesStorage).GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(newStates))

	rootNode, err := storage.GetRootNodeRef(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, n.Ref().Hex(), rootNode.Hex())

	n, _ = node.NewBranchNode(idx, idx, hasher)
	tx2, err := storage.BeginTx(t.Context())
	assert.NoError(t, err)
	err = tx2.InsertNode(t.Context(), n)
	assert.NoError(t, err)
	err = tx1.UpsertRootNodeRef(t.Context(), n.Ref())
	assert.NoError(t, err)
	newStates, err = storage.(*statesStorage).GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(newStates))
	assert.Nil(t, tx2.Commit(t.Context()))
	newStates, err = storage.(*statesStorage).GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, 3, len(newStates))
}

func TestUnimplementedMethods(t *testing.T) {
	stateQueryConext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher, useEIP712)
	assert.NotNil(t, storage)
	storage.(*statesStorage).Close()
}

func TestNodesTxGetNode(t *testing.T) {
	hasher := utxo.NewPoseidonHasher()

	tx := &nodesTx{
		inflightNodes: make(map[core.NodeRef]core.Node),
	}
	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	_, err := tx.getNode(idx)
	assert.EqualError(t, err, core.ErrNotFound.Error())

	n, _ := node.NewLeafNode(node.NewIndexOnly(idx), nil)
	tx.inflightNodes[idx] = n
	n2, err := tx.getNode(idx)
	assert.NoError(t, err)
	assert.Equal(t, n, n2)
}

func TestSetTransactionId(t *testing.T) {
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", "stateQueryContext", "root-schema", "node-schema", hasher, useEIP712)
	storage.SetTransactionId("txid")
	assert.Equal(t, "txid", storage.(*statesStorage).pendingNodesTx.transactionId)
}

func TestGetNewStates(t *testing.T) {
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	s := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", "stateQueryContext", "root-schema", "node-schema", hasher, useEIP712)
	storage := s.(*statesStorage)
	states, err := storage.GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Len(t, states, 0)

	rootNode, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	storage.rootNode = &smtRootNode{
		root: rootNode,
		txId: "txid",
	}
	states, err = storage.GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234567890), hasher)
	node, _ := node.NewLeafNode(node.NewIndexOnly(idx), nil)
	storage.committedNewNodes = map[core.NodeRef]*smtNode{
		idx: {
			node: node,
			txId: "txid",
		},
	}
	states, err = storage.GetNewStates(t.Context())
	assert.NoError(t, err)
	assert.Len(t, states, 2)
}
func TestClose(t *testing.T) {
	stateQueryContext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryContext, "root-schema", "node-schema", hasher, useEIP712)
	// Close should not panic or return error
	storage.Close()
	assert.NotNil(t, storage)
}

func TestGetHasher(t *testing.T) {
	stateQueryContext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryContext, "root-schema", "node-schema", hasher, useEIP712)
	retrievedHasher := storage.(*statesStorage).GetHasher()
	assert.Equal(t, hasher, retrievedHasher)
}

func TestMakeNewStateFromTreeNodeWithBranchNode(t *testing.T) {
	stateQueryContext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryContext, "root-schema", "node-schema", hasher, useEIP712)

	idx1, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	idx2, _ := node.NewNodeIndexFromBigInt(big.NewInt(5678), hasher)
	branchNode, _ := node.NewBranchNode(idx1, idx2, hasher)

	smtNode := &smtNode{
		node: branchNode,
		txId: "test-tx",
	}

	newState, err := storage.(*statesStorage).makeNewStateFromTreeNode(t.Context(), smtNode)
	assert.NoError(t, err)
	assert.NotNil(t, newState)
	assert.Equal(t, "test-tx", newState.TransactionId)
	assert.Equal(t, "node-schema", newState.SchemaId)
}

func TestMakeNewStateFromTreeNodeWithLeafNode(t *testing.T) {
	stateQueryContext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryContext, "root-schema", "node-schema", hasher, useEIP712)

	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(9999), hasher)
	leafNode, _ := node.NewLeafNode(node.NewIndexOnly(idx), nil)

	smtNode := &smtNode{
		node: leafNode,
		txId: "leaf-tx",
	}

	newState, err := storage.(*statesStorage).makeNewStateFromTreeNode(t.Context(), smtNode)
	assert.NoError(t, err)
	assert.NotNil(t, newState)
	assert.Equal(t, "leaf-tx", newState.TransactionId)
}

func TestMakeNewStateFromRootNode(t *testing.T) {
	stateQueryContext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryContext, "root-schema", "node-schema", hasher, useEIP712)

	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(1111), hasher)
	rootRef := idx

	smtRootNode := &smtRootNode{
		root: rootRef,
		txId: "root-tx",
	}

	newState, err := storage.(*statesStorage).makeNewStateFromRootNode(t.Context(), smtRootNode)
	assert.NoError(t, err)
	assert.NotNil(t, newState)
	assert.Equal(t, "root-tx", newState.TransactionId)
	assert.Equal(t, "root-schema", newState.SchemaId)
	assert.Equal(t, "test", storage.(*statesStorage).smtName)
}

func TestHash_EIP712ErrorHandling(t *testing.T) {
	// Test node.Hash_EIP712 with context that might cause issues
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
}

func TestRoot_Hash_EIP712ErrorHandling(t *testing.T) {
	// Test root.Hash_EIP712 with context that might cause issues
	root := &MerkleTreeRoot{
		SmtName:   "test-smt",
		RootIndex: pldtypes.Bytes32{0x01, 0x02, 0x03},
	}

	hash, err := root.Hash_EIP712(t.Context())
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestMakeNewStateFromTreeNodeErrorPath(t *testing.T) {
	// Test with a valid node to ensure no panics occur in error handling
	stateQueryContext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryContext, "root-schema", "node-schema", hasher, useEIP712)

	// Create a branch node with valid structure
	leftChild, _ := node.NewNodeIndexFromBigInt(big.NewInt(100), hasher)
	rightChild, _ := node.NewNodeIndexFromBigInt(big.NewInt(200), hasher)

	branchNode, _ := node.NewBranchNode(leftChild, rightChild, hasher)

	smtNode := &smtNode{
		node: branchNode,
		txId: "branch-tx",
	}

	newState, err := storage.(*statesStorage).makeNewStateFromTreeNode(t.Context(), smtNode)
	assert.NoError(t, err)
	assert.NotNil(t, newState)
	assert.Equal(t, "branch-tx", newState.TransactionId)
}

func TestMakeNewStateFromRootNodeErrorPath(t *testing.T) {
	// Test with a valid root node to ensure proper handling
	stateQueryContext := pldtypes.ShortID()
	hasher := utxo.NewPoseidonHasher()
	useEIP712 := false

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryContext, "root-schema", "node-schema", hasher, useEIP712)

	// Create a root node with valid structure
	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(2222), hasher)

	smtRoot := &smtRootNode{
		root: idx,
		txId: "root-error-tx",
	}

	newState, err := storage.(*statesStorage).makeNewStateFromRootNode(t.Context(), smtRoot)
	assert.NoError(t, err)
	assert.NotNil(t, newState)
	assert.Equal(t, "root-error-tx", newState.TransactionId)
}

func TestGetNewStates_Error1(t *testing.T) {
	s := &statesStorage{
		rootNode: &smtRootNode{
			root: badNodeRef{},
			txId: "txid",
		},
		committedNewNodes: make(map[core.NodeRef]*smtNode),
	}

	states, err := s.GetNewStates(t.Context())
	assert.ErrorContains(t, err, "PD021201: Failed to create new state from committed merkle tree root node. PD021208: Failed to parse root node index.")
	assert.Nil(t, states)
}

func TestGetNewStates_Error2(t *testing.T) {
	hasher := utxo.NewPoseidonHasher()
	rootNode, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)

	s := &statesStorage{
		rootNode: &smtRootNode{
			root: rootNode,
			txId: "txid",
		},
		committedNewNodes: map[core.NodeRef]*smtNode{
			badNodeRef{}: {
				node: badNode{t: core.NodeTypeLeaf},
				txId: "txid2",
			},
		},
	}

	states, err := s.GetNewStates(t.Context())
	assert.ErrorContains(t, err, "PD021202: Failed to create new state from committed merkle tree node. PD021206: Failed to parse node reference.")
	assert.Nil(t, states)
}

func TestMakeNewStateFromTreeNode_Error1(t *testing.T) {
	s := &statesStorage{}
	badNode := &smtNode{
		node: badNode{t: core.NodeTypeLeaf},
		txId: "txid2",
	}

	states, err := s.makeNewStateFromTreeNode(t.Context(), badNode)
	assert.ErrorContains(t, err, "PD021206: Failed to parse node reference.")
	assert.Nil(t, states)
}

func TestMakeNewStateFromTreeNode_Error2(t *testing.T) {
	s := &statesStorage{}
	hasher := utxo.NewPoseidonHasher()
	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(9999), hasher)

	badNode := &smtNode{
		node: badNode{t: core.NodeTypeBranch, r: idx},
		txId: "txid2",
	}

	states, err := s.makeNewStateFromTreeNode(t.Context(), badNode)
	assert.ErrorContains(t, err, "PD021206: Failed to parse node reference.")
	assert.Nil(t, states)
}

func TestMakeNewStateFromTreeNode_Error3(t *testing.T) {
	s := &statesStorage{}
	hasher := utxo.NewPoseidonHasher()
	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(9999), hasher)

	badNode := &smtNode{
		node: badNode{t: core.NodeTypeBranch, r: idx, lc: idx},
		txId: "txid2",
	}

	states, err := s.makeNewStateFromTreeNode(t.Context(), badNode)
	assert.ErrorContains(t, err, "PD021206: Failed to parse node reference.")
	assert.Nil(t, states)
}

func TestMakeNewStateFromTreeNode_Error4(t *testing.T) {
	s := &statesStorage{}
	hasher := utxo.NewPoseidonHasher()
	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(9999), hasher)
	badNode := &smtNode{
		node: badNode{t: core.NodeTypeLeaf, r: idx, i: badIndex{}},
		txId: "txid2",
	}

	states, err := s.makeNewStateFromTreeNode(t.Context(), badNode)
	assert.ErrorContains(t, err, "PD021206: Failed to parse node reference.")
	assert.Nil(t, states)
}

type badNodeRef struct{}

func (b badNodeRef) BigInt() *big.Int           { return nil }
func (b badNodeRef) Hex() string                { return "bad-hex" }
func (b badNodeRef) IsZero() bool               { return false }
func (b badNodeRef) Equal(core.NodeRef) bool    { return false }
func (b badNodeRef) GetHasher() utxocore.Hasher { return nil }

type badNode struct {
	t  core.NodeType
	lc core.NodeRef
	rc core.NodeRef
	r  core.NodeRef
	i  core.NodeIndex
}

func (b badNode) Type() core.NodeType   { return b.t }
func (b badNode) Index() core.NodeIndex { return b.i }
func (b badNode) Value() *big.Int       { return nil }
func (b badNode) Ref() core.NodeRef {
	if b.r != nil {
		return b.r
	}
	return badNodeRef{}
}
func (b badNode) LeftChild() core.NodeRef {
	if b.lc != nil {
		return b.lc
	}
	return badNodeRef{}
}
func (b badNode) RightChild() core.NodeRef {
	if b.rc != nil {
		return b.rc
	}
	return badNodeRef{}
}

type badIndex struct {
	badNodeRef
}

func (b badIndex) IsBitOne(uint) bool { return false }
func (b badIndex) ToPath(int) []bool  { return nil }
