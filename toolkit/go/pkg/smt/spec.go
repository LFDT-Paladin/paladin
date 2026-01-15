package smt

import (
	"context"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/core"
	zetosmt "github.com/LFDT-Paladin/smt/pkg/sparse-merkle-tree/smt"
	utxocore "github.com/LFDT-Paladin/smt/pkg/utxo/core"
)

type MerkleTreeType int

const (
	StatesTree MerkleTreeType = iota
	LockedStatesTree
	KycStatesTree
)

type MerkleTreeSpec struct {
	Name    string
	Levels  int
	Type    MerkleTreeType
	Storage StatesStorage
	Tree    core.SparseMerkleTree
}

func NewMerkleTreeSpec(ctx context.Context, name string, treeType MerkleTreeType, levels int, hasher utxocore.Hasher, callbacks plugintk.DomainCallbacks, merkleTreeRootSchemaId, merkleTreeNodeSchemaId string, stateQueryContext string) (*MerkleTreeSpec, error) {
	var tree core.SparseMerkleTree
	storage := NewStatesStorage(callbacks, name, stateQueryContext, merkleTreeRootSchemaId, merkleTreeNodeSchemaId, hasher)
	tree, err := zetosmt.NewMerkleTree(ctx, storage, levels)
	if err != nil {
		return nil, err
	}
	return &MerkleTreeSpec{
		Name:    name,
		Levels:  levels,
		Type:    treeType,
		Storage: storage,
		Tree:    tree,
	}, nil
}
