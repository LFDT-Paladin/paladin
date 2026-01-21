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

package zeto

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReceiptTransfers(t *testing.T) {
	z := &Zeto{
		coinSchema:           &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"},
		nftSchema:            &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"},
		merkleTreeRootSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ee"},
		merkleTreeNodeSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ef"},
		dataSchema:           &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95f0"},
	}
	ctx := context.Background()

	res1, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		InfoStates:   []*prototk.EndorsableState{},
		InputStates:  []*prototk.EndorsableState{},
		OutputStates: []*prototk.EndorsableState{},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res1.ReceiptJson)
	var receipt1 types.ZetoDomainReceipt
	err = json.Unmarshal([]byte(res1.ReceiptJson), &receipt1)
	require.NoError(t, err)
	assert.Empty(t, receipt1.States.Inputs)
	assert.Empty(t, receipt1.States.Outputs)

	owner1 := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	owner2 := pldtypes.MustParseHexBytes("0x7ddd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8026")

	// Simple mint
	res2, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		InfoStates: []*prototk.EndorsableState{{
			Id:       "0x1a2b3c4d5e6f7081928374655647382910a1b2c3d4e5f6071827364556677889",
			SchemaId: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95f0",
			StateDataJson: `{
				"salt": "0xabcdef",
				"data": "0xdeadbeef"
			}`,
		}},
		InputStates: []*prototk.EndorsableState{},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "0x7980718117603030807695495350922077879582656644717071592146865497574198464253",
			SchemaId: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		}},
	})
	require.NoError(t, err)
	receipt2 := &types.ZetoDomainReceipt{}
	err = json.Unmarshal([]byte(res2.ReceiptJson), receipt2)
	require.NoError(t, err)
	require.Equal(t, pldtypes.MustParseHexBytes("0xdeadbeef"), receipt2.Data)
	assert.Empty(t, receipt2.States.Inputs)
	require.Len(t, receipt2.States.Outputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x7980718117603030807695495350922077879582656644717071592146865497574198464253"), receipt2.States.Outputs[0].ID)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   nil,
		To:     owner1,
		Amount: pldtypes.Int64ToInt256(1),
	}}, receipt2.Transfers)

	// Simple burn
	res3, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "0x7980718117603030807695495350922077879582656644717071592146865497574198464253",
			SchemaId: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{},
	})
	require.NoError(t, err)
	receipt3 := &types.ZetoDomainReceipt{}
	err = json.Unmarshal([]byte(res3.ReceiptJson), receipt3)
	require.NoError(t, err)
	require.Len(t, receipt3.States.Inputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x7980718117603030807695495350922077879582656644717071592146865497574198464253"), receipt3.States.Inputs[0].ID)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     nil,
		Amount: pldtypes.Int64ToInt256(1),
	}}, receipt3.Transfers)

	// Burn with returned remainder
	res4, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "0x7980718117603030807695495350922077879582656644717071592146865497574198464253",
			SchemaId: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{
				"amount": 10,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "0x7980718117603030807695495350922077879582656644717071592146865497574198464254",
			SchemaId: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{
				"amount": 8,
				"owner": "%s"
			}`, owner1),
		}},
	})
	require.NoError(t, err)
	receipt4 := &types.ZetoDomainReceipt{}
	err = json.Unmarshal([]byte(res4.ReceiptJson), receipt4)
	require.NoError(t, err)
	require.Len(t, receipt4.States.Inputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x7980718117603030807695495350922077879582656644717071592146865497574198464253"), receipt4.States.Inputs[0].ID)
	require.Len(t, receipt4.States.Outputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x7980718117603030807695495350922077879582656644717071592146865497574198464254"), receipt4.States.Outputs[0].ID)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     nil,
		Amount: pldtypes.Int64ToInt256(2),
	}}, receipt4.Transfers)

	// Simple transfer
	res5, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "0x7980718117603030807695495350922077879582656644717071592146865497574198464253",
			SchemaId: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "0x7980718117603030807695495350922077879582656644717071592146865497574198464254",
			SchemaId: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner2),
		}},
	})
	require.NoError(t, err)
	receipt5 := &types.ZetoDomainReceipt{}
	err = json.Unmarshal([]byte(res5.ReceiptJson), receipt5)
	require.NoError(t, err)
	require.Len(t, receipt5.States.Inputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x7980718117603030807695495350922077879582656644717071592146865497574198464253"), receipt5.States.Inputs[0].ID)
	require.Len(t, receipt5.States.Outputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x7980718117603030807695495350922077879582656644717071592146865497574198464254"), receipt5.States.Outputs[0].ID)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:   owner1,
		To:     owner2,
		Amount: pldtypes.Int64ToInt256(1),
	}}, receipt5.Transfers)

	// NFT transfer
	res6, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		InputStates: []*prototk.EndorsableState{{
			Id:       "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed",
			StateDataJson: fmt.Sprintf(`{
				"tokenId": "0xdeadbeef",
				"owner": "%s"
			}`, owner1),
		}},
		OutputStates: []*prototk.EndorsableState{{
			Id:       "0x1234567890123456789012345678901234567890123456789012345678901235",
			SchemaId: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed",
			StateDataJson: fmt.Sprintf(`{
				"tokenId": "0xdeadbeef",
				"owner": "%s"
			}`, owner2),
		}},
	})
	require.NoError(t, err)
	receipt6 := &types.ZetoDomainReceipt{}
	err = json.Unmarshal([]byte(res6.ReceiptJson), receipt6)
	require.NoError(t, err)
	require.Len(t, receipt6.States.Inputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x1234567890123456789012345678901234567890123456789012345678901234"), receipt6.States.Inputs[0].ID)
	require.Len(t, receipt6.States.Outputs, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x1234567890123456789012345678901234567890123456789012345678901235"), receipt6.States.Outputs[0].ID)
	assert.ElementsMatch(t, []*types.ReceiptTransfer{{
		From:    owner1,
		To:      owner2,
		TokenId: pldtypes.MustParseHexUint256("0xdeadbeef"),
	}}, receipt6.Transfers)
}

func TestFilterSchema(t *testing.T) {
	ctx := context.Background()

	states := []*prototk.EndorsableState{
		{Id: "state1", SchemaId: "schema1", StateDataJson: "{}"},
		{Id: "state2", SchemaId: "schema2", StateDataJson: "{}"},
		{Id: "state3", SchemaId: "schema1", StateDataJson: "{}"},
		{Id: "state4", SchemaId: "schema3", StateDataJson: "{}"},
	}

	// Filter for schema1
	filtered := filterSchema(states, []string{"schema1"})
	assert.Len(t, filtered, 2)
	assert.Equal(t, "state1", filtered[0].Id)
	assert.Equal(t, "state3", filtered[1].Id)

	// Filter for multiple schemas
	filtered = filterSchema(states, []string{"schema1", "schema3"})
	assert.Len(t, filtered, 3)

	// Filter for non-existent schema
	filtered = filterSchema(states, []string{"schema99"})
	assert.Empty(t, filtered)

	// Filter with empty schemas
	filtered = filterSchema(states, []string{})
	assert.Empty(t, filtered)

	// Filter with empty states
	filtered = filterSchema([]*prototk.EndorsableState{}, []string{"schema1"})
	assert.Empty(t, filtered)

	_ = ctx
}

func TestUnmarshalInfo(t *testing.T) {
	// Valid data
	infoJSON := `{"salt":"0xabcd","data":"0xdeadbeef"}`
	info, err := unmarshalInfo(infoJSON)
	require.NoError(t, err)
	assert.Equal(t, pldtypes.MustParseHexUint256("0xabcd"), info.Salt)
	assert.Equal(t, pldtypes.MustParseHexBytes("0xdeadbeef"), info.Data)

	// Invalid JSON
	_, err = unmarshalInfo("{invalid json")
	assert.Error(t, err)

	// Empty JSON object
	info, err = unmarshalInfo("{}")
	require.NoError(t, err)
	assert.Nil(t, info.Salt)
	assert.Nil(t, info.Data)
}

func TestUnmarshalCoin(t *testing.T) {
	owner := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")

	// Valid coin
	coinJSON := fmt.Sprintf(`{"amount":"100","owner":"%s"}`, owner)
	coin, err := unmarshalCoin(coinJSON)
	require.NoError(t, err)
	assert.Equal(t, pldtypes.Int64ToInt256(100), coin.Amount)
	assert.Equal(t, owner, coin.Owner)

	// Invalid JSON
	_, err = unmarshalCoin("{invalid")
	assert.Error(t, err)

	// Empty object
	coin, err = unmarshalCoin("{}")
	require.NoError(t, err)
	assert.Nil(t, coin.Amount)
	assert.Nil(t, coin.Owner)
}

func TestUnmarshalNFT(t *testing.T) {
	owner := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	tokenID := pldtypes.MustParseHexUint256("0xdeadbeef")

	// Valid NFT
	nftJSON := fmt.Sprintf(`{"tokenId":"%s","owner":"%s"}`, tokenID, owner)
	nft, err := unmarshalNFT(nftJSON)
	require.NoError(t, err)
	assert.Equal(t, tokenID, nft.TokenID)
	assert.Equal(t, owner, nft.Owner)

	// Invalid JSON
	_, err = unmarshalNFT("{invalid")
	assert.Error(t, err)

	// Empty object
	nft, err = unmarshalNFT("{}")
	require.NoError(t, err)
	assert.Nil(t, nft.TokenID)
	assert.Nil(t, nft.Owner)
}

func TestReceiptStates(t *testing.T) {
	ctx := context.Background()

	// Valid states with coin data
	states := []*prototk.EndorsableState{
		{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: `{"amount":"100","owner":"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"}`,
		},
	}

	result, err := receiptStates(ctx, states)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, pldtypes.MustParseHexBytes("0x1234567890123456789012345678901234567890123456789012345678901234"), result[0].ID)
	assert.Equal(t, pldtypes.MustParseBytes32("0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"), result[0].Schema)

	// Invalid state ID
	invalidStates := []*prototk.EndorsableState{
		{
			Id:            "not-a-hex",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: `{}`,
		},
	}

	_, err = receiptStates(ctx, invalidStates)
	assert.Error(t, err)

	// Invalid schema ID
	invalidSchemaStates := []*prototk.EndorsableState{
		{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "not-a-bytes32",
			StateDataJson: `{}`,
		},
	}

	_, err = receiptStates(ctx, invalidSchemaStates)
	assert.Error(t, err)

	// Empty states list
	result, err = receiptStates(ctx, []*prototk.EndorsableState{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestBuildFungibleTransfers_MultipleSenders(t *testing.T) {
	owner1 := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	owner2 := pldtypes.MustParseHexBytes("0x7ddd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8026")

	// Multiple input owners - should return nil
	inputCoins := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner1, Amount: pldtypes.Int64ToInt256(100)},
			{Owner: owner2, Amount: pldtypes.Int64ToInt256(50)},
		},
	}
	outputCoins := &parsedCoins{coins: []*types.ZetoCoin{}}

	transfers, err := buildFungibleTransfers(inputCoins, outputCoins)
	assert.Nil(t, transfers)
	assert.NoError(t, err)
}

func TestBuildFungibleTransfers_MultipleRecipients(t *testing.T) {
	owner1 := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	owner2 := pldtypes.MustParseHexBytes("0x7ddd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8026")
	owner3 := pldtypes.MustParseHexBytes("0x7edd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8027")

	// One sender to multiple recipients
	inputCoins := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner1, Amount: pldtypes.Int64ToInt256(150)},
		},
	}
	outputCoins := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner2, Amount: pldtypes.Int64ToInt256(100)},
			{Owner: owner3, Amount: pldtypes.Int64ToInt256(50)},
		},
	}

	transfers, err := buildFungibleTransfers(inputCoins, outputCoins)
	require.NoError(t, err)
	require.Len(t, transfers, 2)

	// Check both transfers are present with correct amounts
	amounts := make(map[string]*pldtypes.HexUint256)
	for _, transfer := range transfers {
		amounts[transfer.To.String()] = transfer.Amount
		assert.Equal(t, owner1, transfer.From)
	}
	assert.Equal(t, pldtypes.Int64ToInt256(100), amounts[owner2.String()])
	assert.Equal(t, pldtypes.Int64ToInt256(50), amounts[owner3.String()])
}

func TestBuildFungibleTransfers_SelfTransferFiltered(t *testing.T) {
	owner1 := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")

	// Self-transfer with 2 inputs, same amount in outputs (partially)
	inputCoins := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner1, Amount: pldtypes.Int64ToInt256(100)},
		},
	}
	outputCoins := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner1, Amount: pldtypes.Int64ToInt256(60)},
		},
	}

	transfers, err := buildFungibleTransfers(inputCoins, outputCoins)
	require.NoError(t, err)
	require.Len(t, transfers, 1)
	assert.Equal(t, owner1, transfers[0].From)
	assert.Nil(t, transfers[0].To)
	assert.Equal(t, pldtypes.Int64ToInt256(40), transfers[0].Amount)
}

func TestBuildFungibleTransfers_ZeroAmount(t *testing.T) {
	owner1 := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	owner2 := pldtypes.MustParseHexBytes("0x7ddd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8026")

	// Transfer with zero amount should be filtered out
	inputCoins := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner1, Amount: pldtypes.Int64ToInt256(100)},
		},
	}
	outputCoins := &parsedCoins{
		coins: []*types.ZetoCoin{
			{Owner: owner2, Amount: pldtypes.Int64ToInt256(0)},
		},
	}

	transfers, err := buildFungibleTransfers(inputCoins, outputCoins)
	require.NoError(t, err)
	// Zero amount transfer should be filtered out
	require.Empty(t, transfers)
}

func TestBuildNonFungibleTransfers_MultipleTokens(t *testing.T) {
	owner1 := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	owner2 := pldtypes.MustParseHexBytes("0x7ddd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8026")
	tokenID1 := pldtypes.MustParseHexUint256("0xdeadbeef")
	tokenID2 := pldtypes.MustParseHexUint256("0xcafebabe")

	// Multiple NFT transfers
	inputCoins := &parsedCoins{
		nfts: []*types.ZetoNFToken{
			{Owner: owner1, TokenID: tokenID1},
			{Owner: owner1, TokenID: tokenID2},
		},
	}
	outputCoins := &parsedCoins{
		nfts: []*types.ZetoNFToken{
			{Owner: owner2, TokenID: tokenID1},
			{Owner: owner2, TokenID: tokenID2},
		},
	}

	transfers, err := buildNonFungibleTransfers(inputCoins, outputCoins)
	require.NoError(t, err)
	require.Len(t, transfers, 2)

	// Check both tokens are transferred correctly
	for _, transfer := range transfers {
		assert.Equal(t, owner1, transfer.From)
		assert.Equal(t, owner2, transfer.To)
	}
}

func TestBuildNonFungibleTransfers_SelfTransfer(t *testing.T) {
	owner1 := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	tokenID := pldtypes.MustParseHexUint256("0xdeadbeef")

	// Self-transfer (same owner in input and output)
	inputCoins := &parsedCoins{
		nfts: []*types.ZetoNFToken{
			{Owner: owner1, TokenID: tokenID},
		},
	}
	outputCoins := &parsedCoins{
		nfts: []*types.ZetoNFToken{
			{Owner: owner1, TokenID: tokenID},
		},
	}

	transfers, err := buildNonFungibleTransfers(inputCoins, outputCoins)
	require.NoError(t, err)
	// Self-transfers should be skipped, so no To should be set
	require.Len(t, transfers, 1)
	assert.Nil(t, transfers[0].To)
}

func TestBuildNonFungibleTransfers_MismatchedOwners(t *testing.T) {
	owner1 := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	owner2 := pldtypes.MustParseHexBytes("0x7ddd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8026")

	// Multiple input owners - should return nil
	inputCoins := &parsedCoins{
		nfts: []*types.ZetoNFToken{
			{Owner: owner1, TokenID: pldtypes.MustParseHexUint256("0xdeadbeef")},
			{Owner: owner2, TokenID: pldtypes.MustParseHexUint256("0xcafebabe")},
		},
	}
	outputCoins := &parsedCoins{nfts: []*types.ZetoNFToken{}}

	transfers, err := buildNonFungibleTransfers(inputCoins, outputCoins)
	assert.Nil(t, transfers)
	assert.NoError(t, err)
}

func TestParseCoinList_ValidCoins(t *testing.T) {
	z := &Zeto{
		coinSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"},
		nftSchema:  &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"},
	}
	ctx := context.Background()
	owner := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")

	states := []*prototk.EndorsableState{
		{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{"amount":"100","owner":"%s"}`, owner),
		},
		{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901235",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{"amount":"50","owner":"%s"}`, owner),
		},
	}

	result, err := z.parseCoinList(ctx, "inputs", states)
	require.NoError(t, err)
	require.Len(t, result.coins, 2)
	assert.Equal(t, pldtypes.Int64ToInt256(150).Int(), result.total)
}

func TestParseCoinList_ValidNFTs(t *testing.T) {
	z := &Zeto{
		coinSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"},
		nftSchema:  &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"},
	}
	ctx := context.Background()
	owner := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	tokenID := pldtypes.MustParseHexUint256("0xdeadbeef")

	states := []*prototk.EndorsableState{
		{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed",
			StateDataJson: fmt.Sprintf(`{"tokenId":"%s","owner":"%s"}`, tokenID, owner),
		},
	}

	result, err := z.parseCoinList(ctx, "inputs", states)
	require.NoError(t, err)
	require.Len(t, result.nfts, 1)
	assert.Equal(t, tokenID, result.nfts[0].TokenID)
}

func TestParseCoinList_DuplicateStates(t *testing.T) {
	z := &Zeto{
		coinSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"},
		nftSchema:  &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"},
	}
	ctx := context.Background()
	owner := pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")

	states := []*prototk.EndorsableState{
		{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{"amount":"100","owner":"%s"}`, owner),
		},
		{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: fmt.Sprintf(`{"amount":"50","owner":"%s"}`, owner),
		},
	}

	_, err := z.parseCoinList(ctx, "inputs", states)
	assert.Error(t, err)
}

func TestParseCoinList_InvalidJSON(t *testing.T) {
	z := &Zeto{
		coinSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"},
		nftSchema:  &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"},
	}
	ctx := context.Background()

	states := []*prototk.EndorsableState{
		{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: `{invalid json}`,
		},
	}

	_, err := z.parseCoinList(ctx, "inputs", states)
	assert.Error(t, err)
}

func TestParseCoinList_UnexpectedSchema(t *testing.T) {
	z := &Zeto{
		coinSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"},
		nftSchema:  &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"},
	}
	ctx := context.Background()

	states := []*prototk.EndorsableState{
		{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "0x999999999999999999999999999999999999999999999999999999999999999",
			StateDataJson: `{}`,
		},
	}

	_, err := z.parseCoinList(ctx, "inputs", states)
	assert.Error(t, err)
}

func TestBuildReceiptRequest_InfoStateError(t *testing.T) {
	z := &Zeto{
		coinSchema:           &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"},
		nftSchema:            &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"},
		merkleTreeRootSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ee"},
		merkleTreeNodeSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ef"},
		dataSchema:           &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95f0"},
	}
	ctx := context.Background()

	// Invalid info state JSON
	_, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		InfoStates: []*prototk.EndorsableState{{
			Id:            "0x1a2b3c4d5e6f7081928374655647382910a1b2c3d4e5f6071827364556677889",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95f0",
			StateDataJson: `{invalid}`,
		}},
		InputStates:  []*prototk.EndorsableState{},
		OutputStates: []*prototk.EndorsableState{},
	})
	assert.Error(t, err)
}

func TestBuildReceiptRequest_InvalidInputStates(t *testing.T) {
	z := &Zeto{
		coinSchema:           &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"},
		nftSchema:            &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"},
		merkleTreeRootSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ee"},
		merkleTreeNodeSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ef"},
		dataSchema:           &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95f0"},
	}
	ctx := context.Background()

	// Invalid coin data in input states
	_, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		InfoStates: []*prototk.EndorsableState{},
		InputStates: []*prototk.EndorsableState{{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: `{invalid}`,
		}},
		OutputStates: []*prototk.EndorsableState{},
	})
	assert.Error(t, err)
}

func TestBuildReceiptRequest_InvalidOutputStates(t *testing.T) {
	z := &Zeto{
		coinSchema:           &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"},
		nftSchema:            &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ed"},
		merkleTreeRootSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ee"},
		merkleTreeNodeSchema: &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ef"},
		dataSchema:           &prototk.StateSchema{Id: "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95f0"},
	}
	ctx := context.Background()

	// Invalid coin data in output states
	_, err := z.buildReceipt(ctx, &prototk.BuildReceiptRequest{
		InfoStates:  []*prototk.EndorsableState{},
		InputStates: []*prototk.EndorsableState{},
		OutputStates: []*prototk.EndorsableState{{
			Id:            "0x1234567890123456789012345678901234567890123456789012345678901234",
			SchemaId:      "0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec",
			StateDataJson: `{invalid}`,
		}},
	})
	assert.Error(t, err)
}
