// Copyright © 2026 Kaleido, Inc.
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
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeccak256Hasher_Hash_MultipleInputs1(t *testing.T) {
	hasher := &Keccak256Hasher{}
	v1, _ := new(big.Int).SetString("20349940423862035287868699599764962454537984981628200184279725786303353984557", 10)
	v2, _ := new(big.Int).SetString("10955310555638083816119775899206389561202556659568675876759181443512300421331", 10)
	inputs := []*big.Int{
		v1,
		v2,
	}

	result, err := hasher.Hash(inputs)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil hash result")
	}

	expected, _ := new(big.Int).SetString("b7fc494a5f5f4706631139167310f9e2c63b7854e2d25c85026a0626d583880d", 16)
	assert.Equal(t, expected, result)
}

func TestKeccak256Hasher_Hash_MultipleInputs2(t *testing.T) {
	hasher := &Keccak256Hasher{}
	v1 := big.NewInt(10)
	v2, _ := new(big.Int).SetString("43c49e8ba68a9b8a6bb5c230a734d8271a83d2f63722e7651272ebeef5446e", 16)
	v3, _ := new(big.Int).SetString("9198063289874244593808956064764348354864043212453245695133881114917754098693", 10)
	v4, _ := new(big.Int).SetString("3600411115173311692823743444460566395943576560299970643507632418781961416843", 10)

	inputs := []*big.Int{
		v1,
		v2,
		v3,
		v4,
	}

	result, err := hasher.Hash(inputs)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil hash result")
	}

	expected, _ := new(big.Int).SetString("53996271411897636657568875696445326528866233548845021678036249293076239906883", 10)
	assert.Equal(t, expected, result)
}

func TestKeccak256Hasher_Hash_ZeroValue(t *testing.T) {
	hasher := &Keccak256Hasher{}
	inputs := []*big.Int{big.NewInt(0)}

	result, err := hasher.Hash(inputs)
	if err != nil {
		t.Fatalf("Hash() with zero value failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil hash result for zero input")
	}
}

func TestKeccak256Hasher_Hash_LargeNumbers(t *testing.T) {
	hasher := &Keccak256Hasher{}

	largeNum := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 256),
		big.NewInt(1),
	)

	inputs := []*big.Int{largeNum}

	result, err := hasher.Hash(inputs)
	if err != nil {
		t.Fatalf("Hash() with large number failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil hash result for large number")
	}
}

func TestKeccak256Hasher_Hash_EmptySlice(t *testing.T) {
	hasher := &Keccak256Hasher{}
	inputs := []*big.Int{}

	result, err := hasher.Hash(inputs)
	if err != nil {
		t.Fatalf("Hash() with empty slice failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil hash result for empty slice")
	}
}

func TestKeccak256Hasher_CheckInRange(t *testing.T) {
	hasher := &Keccak256Hasher{}

	// Test with a value in range
	inRange := new(big.Int).SetUint64(1234567890)
	if !hasher.CheckInRange(inRange) {
		t.Errorf("CheckInRange() failed for in-range value: %s", inRange.String())
	}

	// Test with a value out of range (greater than 2^256 - 1)
	outOfRange := new(big.Int).Lsh(big.NewInt(1), 256)
	if hasher.CheckInRange(outOfRange) {
		t.Errorf("CheckInRange() should have failed for out-of-range value: %s", outOfRange.String())
	}
}
