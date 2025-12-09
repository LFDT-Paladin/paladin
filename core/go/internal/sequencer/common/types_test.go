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
package common

import (
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestFlushPoint_GetSignerNonce_zeroAddressWithZeroNonce(t *testing.T) {
	fp := &FlushPoint{
		From:  *pldtypes.MustEthAddress("0x0000000000000000000000000000000000000000"),
		Nonce: 0,
	}
	result := fp.GetSignerNonce()
	assert.Equal(t, "0x0000000000000000000000000000000000000000:0", result)
}

func TestFlushPoint_GetSignerNonce_validAddressWithNonce1(t *testing.T) {
	fp := &FlushPoint{
		From:  *pldtypes.MustEthAddress("0xacA6D8Ba6BFf0fa5c8a06A58368CB6097285d5c5"),
		Nonce: 1,
	}
	result := fp.GetSignerNonce()
	assert.Equal(t, "0xaca6d8ba6bff0fa5c8a06a58368cb6097285d5c5:1", result)
}

func TestFlushPoint_GetSignerNonce_validAddressWithHighNonce(t *testing.T) {
	fp := &FlushPoint{
		From:  *pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890"),
		Nonce: 999999,
	}
	result := fp.GetSignerNonce()
	assert.Equal(t, "0x1234567890123456789012345678901234567890:999999", result)
}

func TestFlushPoint_GetSignerNonce_randomAddressWithNonce(t *testing.T) {
	fp := &FlushPoint{
		From:  *pldtypes.RandAddress(),
		Nonce: 42,
	}
	result := fp.GetSignerNonce()
	// For random addresses, verify the format: address:nonce
	assert.Contains(t, result, ":")
	// Verify it contains the nonce at the end
	assert.Contains(t, result, ":42")
	// Verify the address part is a valid hex address format
	assert.Contains(t, result, "0x")
}

func TestTransaction_GetID_newRandomUUID(t *testing.T) {
	id := uuid.New()
	tx := &Transaction{
		ID: id,
	}
	result := tx.GetID()

	// For random UUIDs, verify it's a valid UUID string format
	parsed, err := uuid.Parse(result)
	assert.NoError(t, err)
	assert.Equal(t, id, parsed)
}

func TestTransaction_GetID_specificUUID(t *testing.T) {
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	tx := &Transaction{
		ID: id,
	}
	result := tx.GetID()
	assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000", result)
}

func TestTransaction_GetID_nilUUID(t *testing.T) {
	tx := &Transaction{
		ID: uuid.Nil,
	}
	result := tx.GetID()
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", result)
}
