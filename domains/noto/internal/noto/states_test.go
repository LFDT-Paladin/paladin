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

package noto

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testNullifierContract is the contract the nullifier derivation is bound to in these tests
var testNullifierContract = pldtypes.MustEthAddress("0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3")

func testNullifierNoto() *Noto {
	return &Noto{
		coinSchema:       &prototk.StateSchema{Id: "coin"},
		lockedCoinSchema: &prototk.StateSchema{Id: "lockedCoin"},
		lockInfoSchemaV1: &prototk.StateSchema{Id: "lockInfo"},
	}
}

func testCoinState(id string, coin *types.NotoCoin) *prototk.EndorsableState {
	data, _ := json.Marshal(coin)
	return &prototk.EndorsableState{
		Id:            id,
		SchemaId:      "coin",
		StateDataJson: string(data),
	}
}

func TestEndorsableStateIDs(t *testing.T) {
	ctx := t.Context()
	n := testNullifierNoto()
	owner1 := pldtypes.MustEthAddress("0xbb2b99dde4ca2d4c99f149d13cd55a9edada69eb")
	inputStates := []*prototk.EndorsableState{
		{
			Id:       "1",
			SchemaId: "coin",
			StateDataJson: fmt.Sprintf(`{
				"amount": 1,
				"owner": "%s"
			}`, owner1),
		},
	}

	// Golden vector for keccak256(tag, contract, salt, owner, amount) - changing this changes the
	// on-chain nullifiers of every coin, so it is a breaking change for deployed contracts
	ids := n.endorsableStateIDs(ctx, testNullifierContract, inputStates, true)
	require.Len(t, ids, 1)
	assert.Equal(t, "aaa042edac505aae01d4a35385a26cae21a25702ac393a9397018f8e855075bb", ids[0])

	// Without nullifiers the state ID is used as-is
	ids = n.endorsableStateIDs(ctx, testNullifierContract, inputStates, false)
	require.Len(t, ids, 1)
	assert.Equal(t, "1", ids[0])
}

// States that are not coins have no nullifier - they are identified on-chain by ID
func TestEndorsableStateIDsNonCoinSchema(t *testing.T) {
	ctx := t.Context()
	n := testNullifierNoto()
	states := []*prototk.EndorsableState{
		{
			Id:            "0xaabb",
			SchemaId:      "lockInfo",
			StateDataJson: `{"salt": "0x00", "lockId": "0x01", "owner": "0xbb2b99dde4ca2d4c99f149d13cd55a9edada69eb"}`,
		},
	}
	ids := n.endorsableStateIDs(ctx, testNullifierContract, states, true)
	require.Len(t, ids, 1)
	assert.Equal(t, "0xaabb", ids[0])
}

// Locked states are spent by ID throughout their lifecycle, so they are identified by ID
// even when the caller asks for nullifiers - and a locked coin must never be nullified as
// though it were an unlocked coin, which would silently drop its lockId
func TestEndorsableStateIDsLockedCoinDispatch(t *testing.T) {
	ctx := t.Context()
	n := testNullifierNoto()
	owner := pldtypes.MustEthAddress("0xbb2b99dde4ca2d4c99f149d13cd55a9edada69eb")
	salt := pldtypes.RandBytes32()
	amount := pldtypes.Uint64ToUint256(100)

	lockedCoin := &types.NotoLockedCoin{
		Salt:   salt,
		LockID: pldtypes.RandBytes32(),
		Owner:  owner,
		Amount: amount,
	}
	lockedData, err := json.Marshal(lockedCoin)
	require.NoError(t, err)

	lockedIDs := n.endorsableStateIDs(ctx, testNullifierContract, []*prototk.EndorsableState{
		{Id: "0x01", SchemaId: "lockedCoin", StateDataJson: string(lockedData)},
	}, true)
	require.Len(t, lockedIDs, 1)
	assert.Equal(t, "0x01", lockedIDs[0])

	// A locked coin presented under the unlocked coin schema is rejected rather than being
	// hashed with its lockId dropped
	assert.Nil(t, n.endorsableStateIDs(ctx, testNullifierContract, []*prototk.EndorsableState{
		{Id: "0x03", SchemaId: "coin", StateDataJson: string(lockedData)},
	}, true))
}

// The nullifier must be an injective function of the whole coin. If it were not, a sender
// could build two outputs that differ only by owner - distinct commitments that both the
// notary and the base ledger accept - sharing a single nullifier, then spend their own copy
// to permanently prevent the other owner from spending theirs.
func TestNullifierBindsOwner(t *testing.T) {
	ctx := t.Context()
	salt := pldtypes.RandBytes32()
	amount := pldtypes.Uint64ToUint256(100)
	victim := pldtypes.MustEthAddress("0x1111111111111111111111111111111111111111")
	attacker := pldtypes.MustEthAddress("0x2222222222222222222222222222222222222222")

	toVictim, err := calculateNullifier(ctx, testNullifierContract, &types.NotoCoin{Salt: salt, Owner: victim, Amount: amount})
	require.NoError(t, err)
	toAttacker, err := calculateNullifier(ctx, testNullifierContract, &types.NotoCoin{Salt: salt, Owner: attacker, Amount: amount})
	require.NoError(t, err)
	assert.NotEqual(t, toVictim.String(), toAttacker.String())

	// Sanity check the other fields are covered too
	otherSalt, err := calculateNullifier(ctx, testNullifierContract, &types.NotoCoin{Salt: pldtypes.RandBytes32(), Owner: victim, Amount: amount})
	require.NoError(t, err)
	assert.NotEqual(t, toVictim.String(), otherSalt.String())

	otherAmount, err := calculateNullifier(ctx, testNullifierContract, &types.NotoCoin{Salt: salt, Owner: victim, Amount: pldtypes.Uint64ToUint256(101)})
	require.NoError(t, err)
	assert.NotEqual(t, toVictim.String(), otherAmount.String())

	// Identical coins must nullify identically
	repeat, err := calculateNullifier(ctx, testNullifierContract, &types.NotoCoin{Salt: salt, Owner: victim, Amount: amount})
	require.NoError(t, err)
	assert.Equal(t, toVictim.String(), repeat.String())
}

func TestNullifierIncompleteCoin(t *testing.T) {
	ctx := t.Context()
	amount := pldtypes.Uint64ToUint256(100)
	owner := pldtypes.MustEthAddress("0x1111111111111111111111111111111111111111")

	_, err := calculateNullifier(ctx, testNullifierContract, nil)
	assert.Regexp(t, "PD200044", err)
	_, err = calculateNullifier(ctx, testNullifierContract, &types.NotoCoin{Amount: amount})
	assert.Regexp(t, "PD200044", err)
	_, err = calculateNullifier(ctx, testNullifierContract, &types.NotoCoin{Owner: owner})
	assert.Regexp(t, "PD200044", err)
}

// The same coin data in two different Noto contracts must not derive the same nullifier.
// Nullifier records are keyed per domain rather than per contract (state_nullifiers is keyed
// on domain_name + id, with inserts as OnConflict DoNothing), so a collision would silently
// drop the second record and leave that coin unspendable - while both contracts accept the
// coin on-chain, because their nullifier sets are independent.
func TestNullifierBindsContract(t *testing.T) {
	ctx := t.Context()
	coin := &types.NotoCoin{
		Salt:   pldtypes.RandBytes32(),
		Owner:  pldtypes.MustEthAddress("0x1111111111111111111111111111111111111111"),
		Amount: pldtypes.Uint64ToUint256(100),
	}

	inContractA, err := calculateNullifier(ctx, testNullifierContract, coin)
	require.NoError(t, err)
	inContractB, err := calculateNullifier(ctx, pldtypes.MustEthAddress("0x2222222222222222222222222222222222222222"), coin)
	require.NoError(t, err)
	assert.NotEqual(t, inContractA.String(), inContractB.String())

	// Deriving a nullifier without a contract is refused rather than falling back to an
	// unbound value
	_, err = calculateNullifier(ctx, nil, coin)
	assert.Regexp(t, "PD200047", err)
}

// The payload type is what carries the contract to the owner's node, so it must round-trip
func TestNullifierPayloadTypeRoundTrip(t *testing.T) {
	payloadType := types.NullifierPayloadType(testNullifierContract)
	assert.True(t, types.IsNullifierPayloadType(payloadType))

	parsed, err := types.ParseNullifierPayloadType(payloadType)
	require.NoError(t, err)
	assert.True(t, parsed.Equals(testNullifierContract))

	// An unbound payload type must not resolve to some default contract
	_, err = types.ParseNullifierPayloadType(types.PAYLOAD_DOMAIN_NOTO_NULLIFIER)
	assert.Error(t, err)
	_, err = types.ParseNullifierPayloadType(types.PAYLOAD_DOMAIN_NOTO_NULLIFIER + ":not-an-address")
	assert.Error(t, err)
}
