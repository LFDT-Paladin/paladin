package smt

import (
	"context"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"
	zetosmt "github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/smt"
	utxocore "github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo/core"
)

type MerkleTreeType int

const (
	StatesTree MerkleTreeType = iota
	LockedStatesTree
	KycStatesTree
)

type MerkleTreeSpec struct {
	Name       string
	Levels     int
	Type       MerkleTreeType
	Storage    StatesStorage
	Tree       core.SparseMerkleTree
	EmptyProof *proto.MerkleProof
}

func NewMerkleTreeSpec(ctx context.Context, name string, treeType MerkleTreeType, levels int, hasher utxocore.Hasher, emptyProof *proto.MerkleProof, callbacks plugintk.DomainCallbacks, merkleTreeRootSchemaId, merkleTreeNodeSchemaId string, stateQueryContext string) (*MerkleTreeSpec, error) {
	var tree core.SparseMerkleTree
	storage := NewStatesStorage(callbacks, name, stateQueryContext, merkleTreeRootSchemaId, merkleTreeNodeSchemaId, hasher)
	tree, err := zetosmt.NewMerkleTree(storage, levels)
	if err != nil {
		return nil, err
	}
	return &MerkleTreeSpec{
		Name:       name,
		Levels:     levels,
		Type:       treeType,
		Storage:    storage,
		Tree:       tree,
		EmptyProof: emptyProof,
	}, nil
}
