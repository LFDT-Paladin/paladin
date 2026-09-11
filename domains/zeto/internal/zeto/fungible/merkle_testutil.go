/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fungible

import (
	"context"
	"encoding/json"

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// merkleUTXOCoinOwnerBJJ is the BabyJub owner for merkleUTXOCoinJSON (matches handler_transfer_test SMT fixtures).
const merkleUTXOCoinOwnerBJJ = "0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"

// merkleUTXOCoinJSON hashes to refKey 0x789c99b9... with the shared SMT mock leaf nodes below.
const merkleUTXOCoinJSON = `{"salt":"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec","owner":"` + merkleUTXOCoinOwnerBJJ + `","amount":"0x0f","locked":false}`

func merkleNullifierFindAvailableStates(coinSchemaID string, coinStates []*prototk.StoredState) func(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	data0, _ := json.Marshal(map[string]string{"rootIndex": "0x1234567890123456789012345678901234567890123456789012345678901234"})
	data1, _ := json.Marshal(map[string]string{
		"index": "0x5f5d5e50a650a20986d496e6645ea31770758d924796f0dfc5ac2ad234b03e30",
		"leftChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"refKey": "0x789c99b9a2196addb3ac11567135877e8b86bc9b5f7725808a79757fd36b2a2a",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"type": "0x02",
	})
	data2, _ := json.Marshal(map[string]string{
		"index": "0x8bdc1e9686bc722ac480c60b35090ec521a2d72102b9bbb3043982a138d27514",
		"leftChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"refKey": "0xb2479166472a0635433159a876d6d8f9b904aa0b9249cd1b596750205a2e2c01",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"type": "0x02",
	})
	data3, _ := json.Marshal(map[string]string{
		"index": "0xbc846268f41e264902e0324cc4e1462826c836f902fcead82c18c3d09cb87623",
		"leftChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"refKey": "0xceb5aca5038689895dba9f613a245028f9ea0d135a1b4ceda7e00db6404a0e24",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"type": "0x02",
	})
	count := 0
	return func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		if coinSchemaID != "" && req.SchemaId == coinSchemaID && len(coinStates) > 0 {
			return &prototk.FindAvailableStatesResponse{States: coinStates}, nil
		}
		switch count {
		case 0:
			count++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data0)}}}, nil
		case 1, 4:
			count++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data1)}}}, nil
		case 2, 5:
			count++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data2)}}}, nil
		case 3, 6:
			count++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data3)}}}, nil
		}
		count++
		return &prototk.FindAvailableStatesResponse{}, nil
	}
}

func newMerkleNullifierCallbacks(coinSchemaID string, owner string) *domain.MockDomainCallbacks {
	coinStates := []*prototk.StoredState{{
		Id: "coin-in-merkle-1", DataJson: merkleUTXOCoinJSON, CreatedAt: 1,
	}}
	if owner != merkleUTXOCoinOwnerBJJ {
		coinStates = []*prototk.StoredState{{
			Id: "coin-in-merkle-1",
			DataJson: `{"salt":"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec","owner":"` + owner + `","amount":"0x0f","locked":false}`,
			CreatedAt: 1,
		}}
	}
	return &domain.MockDomainCallbacks{
		MockFindAvailableStates: merkleNullifierFindAvailableStates(coinSchemaID, coinStates),
	}
}

func merkleNullifierKycFindAvailableStates(coinSchemaID string, coinStates []*prototk.StoredState) func(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	data0, _ := json.Marshal(map[string]string{"rootIndex": "0x1234567890123456789012345678901234567890123456789012345678901234"})
	data1, _ := json.Marshal(map[string]string{
		"index": "0x5f5d5e50a650a20986d496e6645ea31770758d924796f0dfc5ac2ad234b03e30",
		"leftChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"refKey": "0x789c99b9a2196addb3ac11567135877e8b86bc9b5f7725808a79757fd36b2a2a",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"type": "0x02",
	})
	data2, _ := json.Marshal(map[string]string{
		"index": "0x8bdc1e9686bc722ac480c60b35090ec521a2d72102b9bbb3043982a138d27514",
		"leftChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"refKey": "0xb2479166472a0635433159a876d6d8f9b904aa0b9249cd1b596750205a2e2c01",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"type": "0x02",
	})
	kycCount := 0
	return func(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
		if coinSchemaID != "" && req.SchemaId == coinSchemaID && len(coinStates) > 0 {
			return &prototk.FindAvailableStatesResponse{States: coinStates}, nil
		}
		switch kycCount {
		case 0, 5:
			kycCount++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data0)}}}, nil
		case 1, 3, 6, 8:
			kycCount++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data1)}}}, nil
		case 2, 4, 7, 9:
			kycCount++
			return &prototk.FindAvailableStatesResponse{States: []*prototk.StoredState{{DataJson: string(data2)}}}, nil
		}
		kycCount++
		return &prototk.FindAvailableStatesResponse{}, nil
	}
}

func newMerkleNullifierKycCallbacks(coinSchemaID string) *domain.MockDomainCallbacks {
	return &domain.MockDomainCallbacks{
		MockFindAvailableStates: merkleNullifierKycFindAvailableStates(coinSchemaID, []*prototk.StoredState{{
			Id: "coin-in-merkle-1", DataJson: merkleUTXOCoinJSON, CreatedAt: 1,
		}}),
	}
}
