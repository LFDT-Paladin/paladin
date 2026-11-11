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

package noto

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newNotoFullSchemaSet(t *testing.T) (context.Context, *domain.MockDomainCallbacks, *Noto) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       testSchema("coin"),
		lockedCoinSchema: testSchema("lockedCoin"),
		lockInfoSchemaV0: testSchema("lockInfo"),
		lockInfoSchemaV1: testSchema("lockInfo_v1"),
		dataSchemaV0:     testSchema("data"),
		dataSchemaV1:     testSchema("data_v1"),
		dataSchemaV2:     testSchema("data_v2"),
		manifestSchema:   testSchema("manifest"),
	}
	return t.Context(), mockCallbacks, n
}

func TestReceiptTransfers(t *testing.T) {
	mockCallbacks := newMockCallbacks()
	n := &Noto{
		Callbacks:        mockCallbacks,
		coinSchema:       testSchema("coin"),
		lockedCoinSchema: testSchema("lockedCoin"),
	}
	ctx := t.Context()

	transfers, err := n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates:  []*prototk.EndorsableState{},
		OutputStates: []*prototk.EndorsableState{},
	})
	require.NoError(t, err)
	require.Len(t, transfers, 0)

	owner1 := pldtypes.MustEthAddress("0xbb2b99dde4ca2d4c99f149d13cd55a9edada69eb")
	owner2 := pldtypes.MustEthAddress("0x3008ee73a70cd1cc57647c7d253a48defe86dd9b")
	owner3 := pldtypes.MustEthAddress("0xea82df8cb94b9fa1a0a711d75a7f7dfdec2ef53c")

	// Simple mint
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: hashName("coin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   nil,
		To:     owner1,
		Amount: pldtypes.Uint64ToUint256(1),
	}}, transfers)

	// Simple burn
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: hashName("coin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     nil,
		Amount: pldtypes.Uint64ToUint256(1),
	}}, transfers)

	// Burn with returned remainder
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: hashName("coin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 10,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "2",
			SchemaId: hashName("coin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 8,
				"owner": "%s"
			}`, owner1),
		}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     nil,
		Amount: pldtypes.Uint64ToUint256(2),
	}}, transfers)

	// Simple transfer
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: hashName("coin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "2",
			SchemaId: hashName("coin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner2),
		}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     owner2,
		Amount: pldtypes.Uint64ToUint256(1),
	}}, transfers)

	// Unlock to multiple recipients, with locked remainder
	transfers, err = n.receiptTransfers(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "1",
			SchemaId: hashName("lockedCoin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 10,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "2",
			SchemaId: hashName("coin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner2),
		}, {
			Id:       "3",
			SchemaId: hashName("coin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner3),
		}, {
			Id:       "4",
			SchemaId: hashName("coin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner3),
		}, {
			Id:       "5",
			SchemaId: hashName("lockedCoin"),
			StateDataJson: fmt.Sprintf(`{
				"amount": 7,
				"owner": "%s"
			}`, owner1),
		}},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     owner2,
		Amount: pldtypes.Uint64ToUint256(1),
	}, {
		From:   owner1,
		To:     owner3,
		Amount: pldtypes.Uint64ToUint256(2),
	}}, transfers)
}

func TestBuildReceiptBadDataState(t *testing.T) {
	ctx, _, n := newNotoFullSchemaSet(t)

	_, err := n.BuildReceipt(ctx, &prototk.BuildReceiptRequest{
		TransactionId: uuid.New().String(),
		InfoStates: []*prototk.EndorsableState{
			{
				Id:            pldtypes.RandBytes32().String(),
				SchemaId:      n.dataSchemaV1.Id,
				StateDataJson: `{! bad data`,
			},
		},
	})
	require.Error(t, err)
}

func TestBuildReceiptBadCoinSchemaId(t *testing.T) {
	n := Noto{
		dataSchemaV0: testSchema("data"),
		dataSchemaV1: testSchema("data_v1"),
		dataSchemaV2: testSchema("data_v2"),
		coinSchema:   &prototk.StateSchema{Id: "not_b32"},
	}

	_, err := n.BuildReceipt(t.Context(), &prototk.BuildReceiptRequest{
		TransactionId: uuid.New().String(),
		InputStates: []*prototk.EndorsableState{
			{
				Id:            pldtypes.RandBytes32().String(),
				SchemaId:      "not_b32",
				StateDataJson: `{}`,
			},
		},
	})
	require.Error(t, err)
}

func TestBuildReceiptBadV0LockInfo(t *testing.T) {
	ctx, _, n := newNotoFullSchemaSet(t)

	_, err := n.BuildReceipt(ctx, &prototk.BuildReceiptRequest{
		TransactionId: uuid.New().String(),
		InfoStates: []*prototk.EndorsableState{
			{
				Id:       pldtypes.RandBytes32().String(),
				SchemaId: n.dataSchemaV0.Id,
				StateDataJson: `{
				  "variant": "0x00"
				}`,
			},
			{
				Id:            pldtypes.RandBytes32().String(),
				SchemaId:      n.lockInfoSchemaV0.Id,
				StateDataJson: `{! bad data`,
			},
		},
		ReadStates: []*prototk.EndorsableState{
			{
				Id:            pldtypes.RandBytes32().String(),
				SchemaId:      n.lockedCoinSchema.Id,
				StateDataJson: `{}`,
			},
		},
	})
	require.Error(t, err)
}

func testGetDomainReceipt(t *testing.T, n *Noto, req *prototk.BuildReceiptRequest) *types.NotoDomainReceipt {
	// Schemas all need mapping

	res, err := n.BuildReceipt(t.Context(), req)
	require.NoError(t, err)

	var notoReceipt types.NotoDomainReceipt
	err = json.Unmarshal([]byte(res.ReceiptJson), &notoReceipt)
	require.NoError(t, err)

	return &notoReceipt
}

func testDataState(t *testing.T, n *Noto, variant pldtypes.HexUint64, data string, from *pldtypes.EthAddress) *prototk.EndorsableState {
	stateData, err := json.Marshal(&types.TransactionData{
		Salt:    pldtypes.RandBytes32(),
		Data:    pldtypes.MustParseHexBytes(data),
		Variant: variant,
		From:    from,
	})
	require.NoError(t, err)
	return &prototk.EndorsableState{
		Id:            pldtypes.RandBytes32().String(),
		SchemaId:      n.dataSchemaV2.Id,
		StateDataJson: string(stateData),
	}
}

func testManifestState(t *testing.T, n *Noto, participant *pldtypes.EthAddress, states ...*prototk.EndorsableState) *prototk.EndorsableState {
	manifest := &types.NotoManifest{Salt: pldtypes.RandBytes32()}
	for _, s := range states {
		manifest.States = append(manifest.States, &types.NotoManifestStateEntry{
			ID:           pldtypes.MustParseBytes32(s.Id),
			Participants: []*pldtypes.EthAddress{participant},
		})
	}
	stateData, err := json.Marshal(manifest)
	require.NoError(t, err)
	return &prototk.EndorsableState{
		Id:            pldtypes.RandBytes32().String(),
		SchemaId:      n.manifestSchema.Id,
		StateDataJson: string(stateData),
	}
}

func mockReverseKeyLookupOwns(mockCallbacks *domain.MockDomainCallbacks, owned *pldtypes.EthAddress) {
	mockCallbacks.MockReverseKeyLookup = func(ctx context.Context, req *prototk.ReverseKeyLookupRequest) (*prototk.ReverseKeyLookupResponse, error) {
		res := &prototk.ReverseKeyLookupResponse{}
		for _, l := range req.Lookups {
			result := &prototk.ReverseKeyLookupResult{Verifier: l.Verifier}
			if l.Verifier == owned.String() {
				result.Found = true
				result.KeyIdentifier = confutil.P("key1")
			}
			res.Results = append(res.Results, result)
		}
		return res, nil
	}
}

func buildTestReceipt(t *testing.T, n *Noto, req *prototk.BuildReceiptRequest) *types.NotoDomainReceipt {
	res, err := n.BuildReceipt(t.Context(), req)
	require.NoError(t, err)
	var receipt types.NotoDomainReceipt
	require.NoError(t, json.Unmarshal([]byte(res.ReceiptJson), &receipt))
	return &receipt
}

func TestBuildReceiptDataWithRequiredStates(t *testing.T) {
	_, mockCallbacks, n := newNotoFullSchemaSet(t)
	me := pldtypes.RandAddress()
	mockReverseKeyLookupOwns(mockCallbacks, me)

	unlockData := testDataState(t, n, types.NotoVariantV2, "0x1111", me)
	txData := testDataState(t, n, types.NotoVariantV2, "0x2222", me)
	manifest := testManifestState(t, n, me, unlockData, txData)

	// Something else in the transaction is unavailable, but not something we need
	receipt := buildTestReceipt(t, n, &prototk.BuildReceiptRequest{
		TransactionId:     uuid.New().String(),
		UnavailableStates: true,
		InfoStates:        []*prototk.EndorsableState{manifest, unlockData, txData},
	})
	assert.Equal(t, pldtypes.MustParseHexBytes("0x2222"), receipt.Data)
	assert.Equal(t, me, receipt.Sender)
}

func TestBuildReceiptOmitsDataWhenRequiredStateUnavailable(t *testing.T) {
	_, mockCallbacks, n := newNotoFullSchemaSet(t)
	me := pldtypes.RandAddress()
	mockReverseKeyLookupOwns(mockCallbacks, me)

	unlockData := testDataState(t, n, types.NotoVariantV2, "0x1111", me)
	txData := testDataState(t, n, types.NotoVariantV2, "0x2222", me)
	manifest := testManifestState(t, n, me, unlockData, txData)

	// The transaction's own data state is unavailable, so the last available data state is the unlock data
	receipt := buildTestReceipt(t, n, &prototk.BuildReceiptRequest{
		TransactionId:     uuid.New().String(),
		UnavailableStates: true,
		InfoStates:        []*prototk.EndorsableState{manifest, unlockData},
	})
	assert.Nil(t, receipt.Data)
	assert.Equal(t, me, receipt.Sender)
}

func TestBuildReceiptOmitsDataWhenManifestUnavailable(t *testing.T) {
	_, _, n := newNotoFullSchemaSet(t)
	me := pldtypes.RandAddress()

	receipt := buildTestReceipt(t, n, &prototk.BuildReceiptRequest{
		TransactionId:     uuid.New().String(),
		UnavailableStates: true,
		InfoStates:        []*prototk.EndorsableState{testDataState(t, n, types.NotoVariantV2, "0x1111", me)},
	})
	assert.Nil(t, receipt.Data)
	assert.Equal(t, me, receipt.Sender)
}

func TestBuildReceiptV0DataWithUnavailableStates(t *testing.T) {
	_, _, n := newNotoFullSchemaSet(t)

	// V0 has no manifest, so we keep the existing behavior
	receipt := buildTestReceipt(t, n, &prototk.BuildReceiptRequest{
		TransactionId:     uuid.New().String(),
		UnavailableStates: true,
		InfoStates:        []*prototk.EndorsableState{testDataState(t, n, types.NotoVariantV0, "0x1111", nil)},
	})
	assert.Equal(t, pldtypes.MustParseHexBytes("0x1111"), receipt.Data)
}

func TestBuildReceiptRequiredStatesLookupFail(t *testing.T) {
	_, mockCallbacks, n := newNotoFullSchemaSet(t)
	me := pldtypes.RandAddress()
	mockCallbacks.MockReverseKeyLookup = func(ctx context.Context, req *prototk.ReverseKeyLookupRequest) (*prototk.ReverseKeyLookupResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	txData := testDataState(t, n, types.NotoVariantV2, "0x2222", me)
	_, err := n.BuildReceipt(t.Context(), &prototk.BuildReceiptRequest{
		TransactionId:     uuid.New().String(),
		UnavailableStates: true,
		InfoStates:        []*prototk.EndorsableState{testManifestState(t, n, me, txData), txData},
	})
	assert.Regexp(t, "pop", err)
}
