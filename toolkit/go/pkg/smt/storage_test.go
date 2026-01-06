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
	hasher := &PoseidonHasher{}

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnCustomError}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	mt, err := smt.NewMerkleTree(storage, 64)
	assert.EqualError(t, err, "test error")
	assert.NotNil(t, storage)
	assert.Nil(t, mt)

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	mt, err = smt.NewMerkleTree(storage, 64)
	assert.NoError(t, err)
	assert.NotNil(t, storage)
	assert.NotNil(t, mt)
	assert.Nil(t, storage.(*statesStorage).rootNode)
	assert.Equal(t, 0, len(storage.(*statesStorage).committedNewNodes))
	newStates, err := storage.(*statesStorage).GetNewStates()
	assert.NoError(t, err)
	assert.Len(t, newStates, 0)
	idx, err := storage.(*statesStorage).GetRootNodeRef()
	assert.NoError(t, err)
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", idx.Hex())

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnBadData}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	mt, err = smt.NewMerkleTree(storage, 64)
	assert.EqualError(t, err, "PD021203: Failed to unmarshal root node index. invalid character 'b' looking for beginning of value")
	assert.NotNil(t, storage)
	assert.Nil(t, mt)

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(0)}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	mt, err = smt.NewMerkleTree(storage, 64)
	assert.NoError(t, err)
	assert.NotNil(t, storage)
	assert.NotNil(t, mt)
	assert.Nil(t, storage.(*statesStorage).pendingNodesTx)

	newStates, err = storage.(*statesStorage).GetNewStates()
	assert.NoError(t, err)
	assert.Empty(t, newStates)
	idx, err = storage.(*statesStorage).GetRootNodeRef()
	assert.NoError(t, err)
	assert.NotEmpty(t, idx)

	// test rollback
	tx, err := storage.BeginTx()
	assert.NoError(t, err)
	idx1, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	err = tx.UpsertRootNodeRef(idx1)
	assert.NoError(t, err)
	assert.Equal(t, "d204000000000000000000000000000000000000000000000000000000000000", storage.(*statesStorage).pendingNodesTx.inflightRoot.Hex())
	assert.Nil(t, tx.Rollback())
	newStates, err = storage.(*statesStorage).GetNewStates()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(newStates))
}

func TestUpsertRootNodeIndex(t *testing.T) {
	stateQueryConext := pldtypes.ShortID()
	hasher := &PoseidonHasher{}

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	_, _ = smt.NewMerkleTree(storage, 64)
	assert.NotNil(t, storage)
	tx, err := storage.BeginTx()
	assert.NoError(t, err)
	idx1, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	err = tx.UpsertRootNodeRef(idx1)
	assert.NoError(t, err)
	assert.Equal(t, "d204000000000000000000000000000000000000000000000000000000000000", storage.(*statesStorage).pendingNodesTx.inflightRoot.Hex())
	assert.Nil(t, tx.Commit())
	newStates, err := storage.(*statesStorage).GetNewStates()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(newStates))

	idx2, err := storage.(*statesStorage).GetRootNodeRef()
	assert.NoError(t, err)
	assert.Equal(t, idx1, idx2)
}

func TestGetNode(t *testing.T) {
	stateQueryConext := pldtypes.ShortID()
	hasher := &PoseidonHasher{}

	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnCustomError}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	_, err := storage.GetNode(idx)
	assert.EqualError(t, err, "test error")

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	_, err = storage.GetNode(idx)
	assert.EqualError(t, err, core.ErrNotFound.Error())

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(1)}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	n, err := storage.GetNode(idx)
	assert.NoError(t, err)
	assert.NotNil(t, n)
	assert.Equal(t, "197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521", n.Index().Hex())
	assert.Equal(t, core.NodeTypeLeaf, n.Type())

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(2)}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	n, err = storage.GetNode(idx)
	assert.NoError(t, err)
	assert.NotNil(t, n)
	assert.Empty(t, n.Index())
	assert.Equal(t, "197b0dc3f167041e03d3eafacec1aa3ab12a0d7a606581af01447c269935e521", n.LeftChild().Hex())

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(3)}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	_, err = storage.GetNode(idx)
	assert.EqualError(t, err, "inputs values not inside Finite Field")

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(4)}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	_, err = storage.GetNode(idx)
	assert.EqualError(t, err, "inputs values not inside Finite Field")

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(5)}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	_, err = storage.GetNode(idx)
	assert.ErrorContains(t, err, "PD021204: Failed to unmarshal Merkle Tree Node from state json. PD020007: Invalid hex")

	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnNode(6)}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	_, err = storage.GetNode(idx)
	assert.ErrorContains(t, err, "PD021204: Failed to unmarshal Merkle Tree Node from state json. PD020008: Failed to parse value as 32 byte hex string")

	// test with committed nodes
	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	tx1, err := storage.BeginTx()
	assert.NoError(t, err)
	n1, _ := node.NewLeafNode(node.NewIndexOnly(idx), nil)
	err = tx1.InsertNode(n1)
	assert.NoError(t, err)
	assert.Nil(t, tx1.Commit())
	n2, err := storage.GetNode(n1.Ref())
	assert.NoError(t, err)
	assert.Equal(t, n1, n2)

	// test with pending nodes (called when we are still updating a leaf node path up to the root)
	storage = NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	tx2, err := storage.BeginTx()
	assert.NoError(t, err)
	n3, _ := node.NewLeafNode(node.NewIndexOnly(idx), nil)
	err = tx2.InsertNode(n3)
	assert.NoError(t, err)
	n4, err := storage.GetNode(n3.Ref())
	assert.NoError(t, err)
	assert.Equal(t, n3, n4)
}

func TestInsertNode(t *testing.T) {
	stateQueryConext := pldtypes.ShortID()
	hasher := &PoseidonHasher{}

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	assert.NotNil(t, storage)
	idx, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	n, _ := node.NewLeafNode(node.NewIndexOnly(idx), nil)

	tx1, err := storage.BeginTx()
	assert.NoError(t, err)
	err = tx1.InsertNode(n)
	assert.NoError(t, err)
	err = tx1.UpsertRootNodeRef(n.Ref())
	assert.NoError(t, err)
	newStates, err := storage.(*statesStorage).GetNewStates()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(newStates))
	assert.Nil(t, tx1.Commit())
	newStates, err = storage.(*statesStorage).GetNewStates()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(newStates))

	rootNode, err := storage.GetRootNodeRef()
	assert.NoError(t, err)
	assert.Equal(t, n.Ref().Hex(), rootNode.Hex())

	n, _ = node.NewBranchNode(idx, idx, hasher)
	tx2, err := storage.BeginTx()
	assert.NoError(t, err)
	err = tx2.InsertNode(n)
	assert.NoError(t, err)
	err = tx1.UpsertRootNodeRef(n.Ref())
	assert.NoError(t, err)
	newStates, err = storage.(*statesStorage).GetNewStates()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(newStates))
	assert.Nil(t, tx2.Commit())
	newStates, err = storage.(*statesStorage).GetNewStates()
	assert.NoError(t, err)
	assert.Equal(t, 3, len(newStates))
}

func TestUnimplementedMethods(t *testing.T) {
	stateQueryConext := pldtypes.ShortID()
	hasher := &PoseidonHasher{}

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", stateQueryConext, "root-schema", "node-schema", hasher)
	assert.NotNil(t, storage)
	storage.(*statesStorage).Close()
}

func TestNodesTxGetNode(t *testing.T) {
	hasher := &PoseidonHasher{}

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
	hasher := &PoseidonHasher{}

	storage := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", "stateQueryContext", "root-schema", "node-schema", hasher)
	storage.SetTransactionId("txid")
	assert.Equal(t, "txid", storage.(*statesStorage).pendingNodesTx.transactionId)
}

func TestGetNewStates(t *testing.T) {
	hasher := &PoseidonHasher{}

	s := NewStatesStorage(&domain.MockDomainCallbacks{MockFindAvailableStates: returnEmptyStates}, "test", "stateQueryContext", "root-schema", "node-schema", hasher)
	storage := s.(*statesStorage)
	states, err := storage.GetNewStates()
	assert.NoError(t, err)
	assert.Len(t, states, 0)

	rootNode, _ := node.NewNodeIndexFromBigInt(big.NewInt(1234), hasher)
	storage.rootNode = &smtRootNode{
		root: rootNode,
		txId: "txid",
	}
	states, err = storage.GetNewStates()
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
	states, err = storage.GetNewStates()
	assert.NoError(t, err)
	assert.Len(t, states, 2)
}
