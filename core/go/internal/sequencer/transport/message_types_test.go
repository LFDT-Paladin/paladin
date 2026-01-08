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

package transport

import (
	"encoding/json"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== ParseCoordinatorHeartbeatNotification Tests =====

func TestParseCoordinatorHeartbeatNotification_Success(t *testing.T) {
	from := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	snapshot := common.CoordinatorSnapshot{
		CoordinatorState:       "Idle",
		BlockHeight:            100,
		FlushPoints:            []*common.FlushPoint{},
		PooledTransactions:     []*common.Transaction{},
		DispatchedTransactions: []*common.DispatchedTransaction{},
		ConfirmedTransactions:  []*common.ConfirmedTransaction{},
	}

	notification := CoordinatorHeartbeatNotification{
		From:                from,
		ContractAddress:     contractAddress,
		CoordinatorSnapshot: snapshot,
	}

	bytes, err := json.Marshal(notification)
	require.NoError(t, err)

	parsed, err := ParseCoordinatorHeartbeatNotification(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, from, parsed.From)
	assert.NotNil(t, parsed.ContractAddress)
	assert.Equal(t, contractAddress.HexString(), parsed.ContractAddress.HexString())
	assert.Equal(t, snapshot.CoordinatorState, parsed.CoordinatorState)
	assert.Equal(t, snapshot.BlockHeight, parsed.BlockHeight)
}

func TestParseCoordinatorHeartbeatNotification_WithFlushPoints(t *testing.T) {
	from := "coordinator-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	flushPointAddr := pldtypes.MustEthAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	flushPointHash := pldtypes.MustParseBytes32("0x00000000000000000000000000000000000000000000000000000000000000ab")
	txID := uuid.New()

	flushPoint := &common.FlushPoint{
		From:          *flushPointAddr,
		Nonce:         42,
		TransactionID: txID,
		Hash:          flushPointHash,
		Confirmed:     true,
	}

	snapshot := common.CoordinatorSnapshot{
		CoordinatorState:       "Active",
		BlockHeight:            200,
		FlushPoints:            []*common.FlushPoint{flushPoint},
		PooledTransactions:     []*common.Transaction{},
		DispatchedTransactions: []*common.DispatchedTransaction{},
		ConfirmedTransactions:  []*common.ConfirmedTransaction{},
	}

	notification := CoordinatorHeartbeatNotification{
		From:                from,
		ContractAddress:     contractAddress,
		CoordinatorSnapshot: snapshot,
	}

	bytes, err := json.Marshal(notification)
	require.NoError(t, err)

	parsed, err := ParseCoordinatorHeartbeatNotification(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, 1, len(parsed.FlushPoints))
	assert.Equal(t, flushPointAddr.HexString(), parsed.FlushPoints[0].From.HexString())
	assert.Equal(t, uint64(42), parsed.FlushPoints[0].Nonce)
	assert.Equal(t, txID, parsed.FlushPoints[0].TransactionID)
}

func TestParseCoordinatorHeartbeatNotification_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json syntax}`)

	parsed, err := ParseCoordinatorHeartbeatNotification(invalidJSON)
	assert.Error(t, err)
	assert.NotNil(t, parsed) // Function returns struct even on error
}

func TestParseCoordinatorHeartbeatNotification_EmptyJSON(t *testing.T) {
	emptyJSON := []byte(`{}`)

	parsed, err := ParseCoordinatorHeartbeatNotification(emptyJSON)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, "", parsed.From)
	assert.Nil(t, parsed.ContractAddress)
}

func TestParseCoordinatorHeartbeatNotification_NilContractAddress(t *testing.T) {
	from := "coordinator-node"
	snapshot := common.CoordinatorSnapshot{
		CoordinatorState: "Idle",
		BlockHeight:      100,
	}

	notification := CoordinatorHeartbeatNotification{
		From:                from,
		ContractAddress:     nil,
		CoordinatorSnapshot: snapshot,
	}

	bytes, err := json.Marshal(notification)
	require.NoError(t, err)

	parsed, err := ParseCoordinatorHeartbeatNotification(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Nil(t, parsed.ContractAddress)
}

// ===== ParseTransactionRequest Tests =====

func TestParseTransactionRequest_Success(t *testing.T) {
	sender := "sender-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID1 := uuid.New()
	txID2 := uuid.New()

	transactions := []*components.PrivateTransaction{
		{
			ID:      txID1,
			Domain:  "test-domain",
			Address: *contractAddress,
		},
		{
			ID:      txID2,
			Domain:  "test-domain",
			Address: *contractAddress,
		},
	}

	request := TransactionRequest{
		Sender:          sender,
		ContractAddress: contractAddress,
		Transactions:    transactions,
	}

	bytes, err := json.Marshal(request)
	require.NoError(t, err)

	parsed, err := ParseTransactionRequest(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, sender, parsed.Sender)
	assert.NotNil(t, parsed.ContractAddress)
	assert.Equal(t, contractAddress.HexString(), parsed.ContractAddress.HexString())
	assert.Equal(t, 2, len(parsed.Transactions))
	assert.Equal(t, txID1, parsed.Transactions[0].ID)
	assert.Equal(t, txID2, parsed.Transactions[1].ID)
}

func TestParseTransactionRequest_EmptyTransactions(t *testing.T) {
	sender := "sender-node"
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")

	request := TransactionRequest{
		Sender:          sender,
		ContractAddress: contractAddress,
		Transactions:    []*components.PrivateTransaction{},
	}

	bytes, err := json.Marshal(request)
	require.NoError(t, err)

	parsed, err := ParseTransactionRequest(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, sender, parsed.Sender)
	assert.Equal(t, 0, len(parsed.Transactions))
}

func TestParseTransactionRequest_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json syntax}`)

	parsed, err := ParseTransactionRequest(invalidJSON)
	assert.Error(t, err)
	assert.NotNil(t, parsed) // Function returns struct even on error
}

func TestParseTransactionRequest_EmptyJSON(t *testing.T) {
	emptyJSON := []byte(`{}`)

	parsed, err := ParseTransactionRequest(emptyJSON)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, "", parsed.Sender)
	assert.Nil(t, parsed.ContractAddress)
	assert.Nil(t, parsed.Transactions)
}

func TestParseTransactionRequest_NilContractAddress(t *testing.T) {
	sender := "sender-node"

	request := TransactionRequest{
		Sender:          sender,
		ContractAddress: nil,
		Transactions:    []*components.PrivateTransaction{},
	}

	bytes, err := json.Marshal(request)
	require.NoError(t, err)

	parsed, err := ParseTransactionRequest(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Nil(t, parsed.ContractAddress)
}

// ===== ParseDispatchConfirmationRequest Tests =====

func TestParseDispatchConfirmationRequest_Success(t *testing.T) {
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinator := "coordinator-node"
	txID := uuid.New()
	txHash := []byte{0x01, 0x02, 0x03, 0x04}

	request := DispatchConfirmationRequest{
		ContractAddress: contractAddress,
		Coordinator:     coordinator,
		TransactionID:   txID,
		TransactionHash: txHash,
	}

	bytes, err := json.Marshal(request)
	require.NoError(t, err)

	parsed, err := ParseDispatchConfirmationRequest(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.NotNil(t, parsed.ContractAddress)
	assert.Equal(t, contractAddress.HexString(), parsed.ContractAddress.HexString())
	assert.Equal(t, coordinator, parsed.Coordinator)
	assert.Equal(t, txID, parsed.TransactionID)
	assert.Equal(t, txHash, parsed.TransactionHash)
}

func TestParseDispatchConfirmationRequest_EmptyTransactionHash(t *testing.T) {
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	coordinator := "coordinator-node"
	txID := uuid.New()

	request := DispatchConfirmationRequest{
		ContractAddress: contractAddress,
		Coordinator:     coordinator,
		TransactionID:   txID,
		TransactionHash: []byte{},
	}

	bytes, err := json.Marshal(request)
	require.NoError(t, err)

	parsed, err := ParseDispatchConfirmationRequest(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, 0, len(parsed.TransactionHash))
}

func TestParseDispatchConfirmationRequest_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json syntax}`)

	parsed, err := ParseDispatchConfirmationRequest(invalidJSON)
	assert.Error(t, err)
	assert.NotNil(t, parsed) // Function returns struct even on error
}

func TestParseDispatchConfirmationRequest_EmptyJSON(t *testing.T) {
	emptyJSON := []byte(`{}`)

	parsed, err := ParseDispatchConfirmationRequest(emptyJSON)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Nil(t, parsed.ContractAddress)
	assert.Equal(t, "", parsed.Coordinator)
	assert.Equal(t, uuid.Nil, parsed.TransactionID)
	assert.Nil(t, parsed.TransactionHash)
}

func TestParseDispatchConfirmationRequest_NilContractAddress(t *testing.T) {
	coordinator := "coordinator-node"
	txID := uuid.New()
	txHash := []byte{0x01, 0x02, 0x03, 0x04}

	request := DispatchConfirmationRequest{
		ContractAddress: nil,
		Coordinator:     coordinator,
		TransactionID:   txID,
		TransactionHash: txHash,
	}

	bytes, err := json.Marshal(request)
	require.NoError(t, err)

	parsed, err := ParseDispatchConfirmationRequest(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Nil(t, parsed.ContractAddress)
}

// ===== ParseDispatchConfirmationResponse Tests =====

func TestParseDispatchConfirmationResponse_Success(t *testing.T) {
	contractAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	txID := uuid.New()

	response := DispatchConfirmationResponse{
		ContractAddress: contractAddress,
		TransactionID:   txID,
	}

	bytes, err := json.Marshal(response)
	require.NoError(t, err)

	parsed, err := ParseDispatchConfirmationResponse(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.NotNil(t, parsed.ContractAddress)
	assert.Equal(t, contractAddress.HexString(), parsed.ContractAddress.HexString())
	assert.Equal(t, txID, parsed.TransactionID)
}

func TestParseDispatchConfirmationResponse_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json syntax}`)

	parsed, err := ParseDispatchConfirmationResponse(invalidJSON)
	assert.Error(t, err)
	assert.NotNil(t, parsed) // Function returns struct even on error
}

func TestParseDispatchConfirmationResponse_EmptyJSON(t *testing.T) {
	emptyJSON := []byte(`{}`)

	parsed, err := ParseDispatchConfirmationResponse(emptyJSON)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Nil(t, parsed.ContractAddress)
	assert.Equal(t, uuid.Nil, parsed.TransactionID)
}

func TestParseDispatchConfirmationResponse_NilContractAddress(t *testing.T) {
	txID := uuid.New()

	response := DispatchConfirmationResponse{
		ContractAddress: nil,
		TransactionID:   txID,
	}

	bytes, err := json.Marshal(response)
	require.NoError(t, err)

	parsed, err := ParseDispatchConfirmationResponse(bytes)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Nil(t, parsed.ContractAddress)
	assert.Equal(t, txID, parsed.TransactionID)
}
