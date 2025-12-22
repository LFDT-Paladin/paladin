// Copyright © 2025 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package smt

import (
	"encoding/json"
	"math/big"
	"strconv"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"golang.org/x/crypto/sha3"
)

type Keccak256Hasher struct{}

func (k *Keccak256Hasher) Hash(inputs []*big.Int) (*big.Int, error) {
	paramTypes := abi.ParameterArray{}
	paramValues := map[string]any{}
	for i, input := range inputs {
		paramTypes = append(paramTypes, &abi.Parameter{
			Type: "uint256",
			Name: strconv.Itoa(i),
		})
		paramValues[strconv.Itoa(i)] = "0x" + input.Text(16)
	}

	jsonData, err := json.Marshal(paramValues)
	if err != nil {
		return nil, err
	}

	encoded, err := paramTypes.EncodeABIDataJSON(jsonData)
	if err != nil {
		return nil, err
	}
	hash := sha3.NewLegacyKeccak256()
	hash.Write(encoded)
	h32 := make([]byte, 32)
	h := hash.Sum(h32)
	_hash := new(big.Int).SetBytes(h)
	return _hash, nil
}

func (k *Keccak256Hasher) CheckInRange(a *big.Int) bool {
	// The range is [0, 2^256 - 1] for Keccak256 inputs
	max := new(big.Int).Lsh(big.NewInt(1), 256)
	return a.Cmp(big.NewInt(0)) >= 0 && a.Cmp(max) < 0
}
