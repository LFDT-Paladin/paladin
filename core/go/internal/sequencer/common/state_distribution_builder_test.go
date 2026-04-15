/*
 * Copyright © 2026 Kaleido, Inc.
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
package common

import (
	"context"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStateDistributionBuilder(t *testing.T) {
	tx := &components.PrivateTransaction{}
	builder := NewStateDistributionBuilder("node1", tx)

	assert.Equal(t, "node1", builder.LocalNode)
	assert.Equal(t, tx, builder.tx)
	assert.Empty(t, builder.Remote)
	assert.Empty(t, builder.Local)
}

func TestBuild_NilPostAssembly(t *testing.T) {
	ctx := context.Background()
	tx := &components.PrivateTransaction{
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: nil,
	}
	builder := NewStateDistributionBuilder("node1", tx)

	_, err := builder.Build(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012634")
}

func TestBuild_MismatchedOutputStates(t *testing.T) {
	ctx := context.Background()
	tx := &components.PrivateTransaction{
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{{}, {}},
			OutputStates:          []*components.FullState{{}},
			InfoStatesPotential:   []*prototk.NewState{},
			InfoStates:            []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	_, err := builder.Build(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012634")
}

func TestBuild_MismatchedInfoStates(t *testing.T) {
	ctx := context.Background()
	tx := &components.PrivateTransaction{
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{},
			OutputStates:          []*components.FullState{},
			InfoStatesPotential:   []*prototk.NewState{{}, {}},
			InfoStates:            []*components.FullState{{}},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	_, err := builder.Build(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012634")
}

func TestBuild_InvalidFromLocator(t *testing.T) {
	ctx := context.Background()
	tx := &components.PrivateTransaction{
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice", // missing node
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{},
			OutputStates:          []*components.FullState{},
			InfoStatesPotential:   []*prototk.NewState{},
			InfoStates:            []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	_, err := builder.Build(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012633")
}

func TestBuild_LocalDistribution(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"alice@node1"},
				},
			},
			OutputStates: []*components.FullState{
				{
					ID:     stateID,
					Schema: schemaID,
					Data:   pldtypes.RawJSON(`{"key":"value"}`),
				},
			},
			InfoStatesPotential: []*prototk.NewState{},
			InfoStates:          []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	result, err := builder.Build(ctx)
	require.NoError(t, err)

	assert.Equal(t, "node1", result.LocalNode)
	assert.Equal(t, "node1", result.OriginatorNode)
	assert.Len(t, result.Local, 1)
	assert.Empty(t, result.Remote)

	assert.Equal(t, "alice@node1", result.Local[0].IdentityLocator)
	assert.Equal(t, "test-domain", result.Local[0].Domain)
	assert.Equal(t, stateID.String(), result.Local[0].StateID)
	assert.Equal(t, schemaID.String(), result.Local[0].SchemaID)
}

func TestBuild_RemoteDistribution(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"bob@node2"},
				},
			},
			OutputStates: []*components.FullState{
				{
					ID:     stateID,
					Schema: schemaID,
					Data:   pldtypes.RawJSON(`{"key":"value"}`),
				},
			},
			InfoStatesPotential: []*prototk.NewState{},
			InfoStates:          []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	result, err := builder.Build(ctx)
	require.NoError(t, err)

	// Originator is added automatically
	assert.Len(t, result.Local, 1)
	assert.Len(t, result.Remote, 1)

	assert.Equal(t, "bob@node2", result.Remote[0].IdentityLocator)
	assert.Equal(t, "alice@node1", result.Local[0].IdentityLocator)
}

func TestBuild_OriginatorAddedToDistributionList(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"bob@node2"}, // originator not included
				},
			},
			OutputStates: []*components.FullState{
				{
					ID:     stateID,
					Schema: schemaID,
					Data:   pldtypes.RawJSON(`{}`),
				},
			},
			InfoStatesPotential: []*prototk.NewState{},
			InfoStates:          []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	result, err := builder.Build(ctx)
	require.NoError(t, err)

	// Originator alice@node1 should be added automatically
	assert.Len(t, result.Local, 1)
	assert.Equal(t, "alice@node1", result.Local[0].IdentityLocator)
}

func TestBuild_OriginatorAlreadyInDistributionList(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"alice@node1", "bob@node2"},
				},
			},
			OutputStates: []*components.FullState{
				{
					ID:     stateID,
					Schema: schemaID,
					Data:   pldtypes.RawJSON(`{}`),
				},
			},
			InfoStatesPotential: []*prototk.NewState{},
			InfoStates:          []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	result, err := builder.Build(ctx)
	require.NoError(t, err)

	// Originator should not be duplicated
	assert.Len(t, result.Local, 1)
	assert.Len(t, result.Remote, 1)
}

func TestBuild_WithNullifierSpec(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"alice@node1"},
					NullifierSpecs: []*prototk.NullifierSpec{
						{
							Party:        "alice@node1",
							Algorithm:    "snark",
							VerifierType: "groth16",
							PayloadType:  "json",
						},
					},
				},
			},
			OutputStates: []*components.FullState{
				{
					ID:     stateID,
					Schema: schemaID,
					Data:   pldtypes.RawJSON(`{}`),
				},
			},
			InfoStatesPotential: []*prototk.NewState{},
			InfoStates:          []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	result, err := builder.Build(ctx)
	require.NoError(t, err)

	require.Len(t, result.Local, 1)
	assert.NotNil(t, result.Local[0].NullifierAlgorithm)
	assert.Equal(t, "snark", *result.Local[0].NullifierAlgorithm)
	assert.NotNil(t, result.Local[0].NullifierVerifierType)
	assert.Equal(t, "groth16", *result.Local[0].NullifierVerifierType)
	assert.NotNil(t, result.Local[0].NullifierPayloadType)
	assert.Equal(t, "json", *result.Local[0].NullifierPayloadType)
}

func TestBuild_NullifierSpecNotInDistributionList(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"alice@node1"},
					NullifierSpecs: []*prototk.NullifierSpec{
						{
							Party:     "charlie@node3", // not in distribution list
							Algorithm: "snark",
						},
					},
				},
			},
			OutputStates: []*components.FullState{
				{
					ID:     stateID,
					Schema: schemaID,
					Data:   pldtypes.RawJSON(`{}`),
				},
			},
			InfoStatesPotential: []*prototk.NewState{},
			InfoStates:          []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	_, err := builder.Build(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012636")
}

func TestBuild_InvalidRecipientLocator(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"bob"}, // missing node
				},
			},
			OutputStates: []*components.FullState{
				{
					ID:     stateID,
					Schema: schemaID,
					Data:   pldtypes.RawJSON(`{}`),
				},
			},
			InfoStatesPotential: []*prototk.NewState{},
			InfoStates:          []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	_, err := builder.Build(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012635")
}

func TestBuild_InfoStates(t *testing.T) {
	ctx := context.Background()
	outputStateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	infoStateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{DistributionList: []string{"alice@node1"}},
			},
			OutputStates: []*components.FullState{
				{ID: outputStateID, Schema: schemaID, Data: pldtypes.RawJSON(`{}`)},
			},
			InfoStatesPotential: []*prototk.NewState{
				{DistributionList: []string{"bob@node2"}},
			},
			InfoStates: []*components.FullState{
				{ID: infoStateID, Schema: schemaID, Data: pldtypes.RawJSON(`{}`)},
			},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	result, err := builder.Build(ctx)
	require.NoError(t, err)

	// 2 local (alice for output + alice for info since she's originator)
	assert.Len(t, result.Local, 2)
	// 1 remote (bob for info)
	assert.Len(t, result.Remote, 1)
}

func TestBuild_MultipleOutputStates(t *testing.T) {
	ctx := context.Background()
	stateID1 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	stateID2 := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{DistributionList: []string{"alice@node1"}},
				{DistributionList: []string{"bob@node2"}},
			},
			OutputStates: []*components.FullState{
				{ID: stateID1, Schema: schemaID, Data: pldtypes.RawJSON(`{}`)},
				{ID: stateID2, Schema: schemaID, Data: pldtypes.RawJSON(`{}`)},
			},
			InfoStatesPotential: []*prototk.NewState{},
			InfoStates:          []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	result, err := builder.Build(ctx)
	require.NoError(t, err)

	// alice@node1 for state1 + alice@node1 for state2 (originator added)
	assert.Len(t, result.Local, 2)
	// bob@node2 for state2
	assert.Len(t, result.Remote, 1)
}

func TestBuild_MultipleNullifierSpecsOnlyFirstMatches(t *testing.T) {
	ctx := context.Background()
	stateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{
					DistributionList: []string{"alice@node1", "bob@node2"},
					NullifierSpecs: []*prototk.NullifierSpec{
						{Party: "alice@node1", Algorithm: "alg1"},
						{Party: "bob@node2", Algorithm: "alg2"},
					},
				},
			},
			OutputStates: []*components.FullState{
				{ID: stateID, Schema: schemaID, Data: pldtypes.RawJSON(`{}`)},
			},
			InfoStatesPotential: []*prototk.NewState{},
			InfoStates:          []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	result, err := builder.Build(ctx)
	require.NoError(t, err)

	require.Len(t, result.Local, 1)
	require.Len(t, result.Remote, 1)

	assert.Equal(t, "alg1", *result.Local[0].NullifierAlgorithm)
	assert.Equal(t, "alg2", *result.Remote[0].NullifierAlgorithm)
}

func TestBuild_EmptyStates(t *testing.T) {
	ctx := context.Background()

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{},
			OutputStates:          []*components.FullState{},
			InfoStatesPotential:   []*prototk.NewState{},
			InfoStates:            []*components.FullState{},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	result, err := builder.Build(ctx)
	require.NoError(t, err)

	assert.Empty(t, result.Local)
	assert.Empty(t, result.Remote)
	assert.Equal(t, "node1", result.OriginatorNode)
}

func TestBuild_InfoStateProcessError(t *testing.T) {
	ctx := context.Background()
	outputStateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	infoStateID := pldtypes.HexBytes(pldtypes.RandBytes(32))
	schemaID := pldtypes.Bytes32(pldtypes.RandBytes(32))

	tx := &components.PrivateTransaction{
		Domain:  "test-domain",
		Address: *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice@node1",
			},
		},
		PostAssembly: &components.TransactionPostAssembly{
			OutputStatesPotential: []*prototk.NewState{
				{DistributionList: []string{"alice@node1"}},
			},
			OutputStates: []*components.FullState{
				{ID: outputStateID, Schema: schemaID, Data: pldtypes.RawJSON(`{}`)},
			},
			InfoStatesPotential: []*prototk.NewState{
				{DistributionList: []string{"bob"}}, // missing node - triggers error in processStateForDistribution
			},
			InfoStates: []*components.FullState{
				{ID: infoStateID, Schema: schemaID, Data: pldtypes.RawJSON(`{}`)},
			},
		},
	}
	builder := NewStateDistributionBuilder("node1", tx)

	_, err := builder.Build(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PD012635")
}
