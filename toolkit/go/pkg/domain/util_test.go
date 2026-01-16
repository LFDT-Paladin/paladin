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

package domain

import (
	"testing"

	pb "github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestFindVerifierFound(t *testing.T) {
	verifiers := []*pb.ResolvedVerifier{
		{
			Lookup:       "lookup1",
			Algorithm:    "algorithm1",
			VerifierType: "type1",
		},
		{
			Lookup:       "lookup2",
			Algorithm:    "algorithm2",
			VerifierType: "type2",
		},
	}

	result := FindVerifier("lookup1", "algorithm1", "type1", verifiers)
	assert.NotNil(t, result)
	assert.Equal(t, "lookup1", result.Lookup)
	assert.Equal(t, "algorithm1", result.Algorithm)
	assert.Equal(t, "type1", result.VerifierType)
}

func TestFindVerifierNotFound(t *testing.T) {
	verifiers := []*pb.ResolvedVerifier{
		{
			Lookup:       "lookup1",
			Algorithm:    "algorithm1",
			VerifierType: "type1",
		},
	}

	result := FindVerifier("nonexistent", "algorithm1", "type1", verifiers)
	assert.Nil(t, result)
}

func TestFindVerifierEmptyList(t *testing.T) {
	result := FindVerifier("lookup1", "algorithm1", "type1", []*pb.ResolvedVerifier{})
	assert.Nil(t, result)
}

func TestFindVerifierPartialMatch(t *testing.T) {
	verifiers := []*pb.ResolvedVerifier{
		{
			Lookup:       "lookup1",
			Algorithm:    "algorithm1",
			VerifierType: "type1",
		},
	}

	// Matching lookup but different algorithm
	result := FindVerifier("lookup1", "different_algorithm", "type1", verifiers)
	assert.Nil(t, result)

	// Matching lookup and algorithm but different type
	result = FindVerifier("lookup1", "algorithm1", "different_type", verifiers)
	assert.Nil(t, result)

	// Matching algorithm but different lookup
	result = FindVerifier("different_lookup", "algorithm1", "type1", verifiers)
	assert.Nil(t, result)
}

func TestFindVerifierMultipleMatches(t *testing.T) {
	verifiers := []*pb.ResolvedVerifier{
		{
			Lookup:       "lookup1",
			Algorithm:    "algorithm1",
			VerifierType: "type1",
		},
		{
			Lookup:       "lookup1",
			Algorithm:    "algorithm1",
			VerifierType: "type1",
		},
	}

	// Should return the first match
	result := FindVerifier("lookup1", "algorithm1", "type1", verifiers)
	assert.Equal(t, verifiers[0], result)
}

func TestFindVerifierCaseSensitive(t *testing.T) {
	verifiers := []*pb.ResolvedVerifier{
		{
			Lookup:       "Lookup1",
			Algorithm:    "Algorithm1",
			VerifierType: "Type1",
		},
	}

	result := FindVerifier("lookup1", "algorithm1", "type1", verifiers)
	assert.Nil(t, result, "should be case-sensitive")
}

func TestFindVerifierLargeList(t *testing.T) {
	verifiers := make([]*pb.ResolvedVerifier, 1000)
	for i := 0; i < 1000; i++ {
		lookup := "lookup"
		if i == 500 {
			lookup = "target"
		}
		verifiers[i] = &pb.ResolvedVerifier{
			Lookup:       lookup,
			Algorithm:    "algorithm",
			VerifierType: "type",
		}
	}

	result := FindVerifier("target", "algorithm", "type", verifiers)
	assert.NotNil(t, result)
	assert.Equal(t, "target", result.Lookup)
}

func TestFindAttestationFound(t *testing.T) {
	attestations := []*pb.AttestationResult{
		{Name: "attestation1"},
		{Name: "attestation2"},
		{Name: "attestation3"},
	}

	result := FindAttestation("attestation2", attestations)
	assert.NotNil(t, result)
	assert.Equal(t, "attestation2", result.Name)
}

func TestFindAttestationNotFound(t *testing.T) {
	attestations := []*pb.AttestationResult{
		{Name: "attestation1"},
		{Name: "attestation2"},
	}

	result := FindAttestation("nonexistent", attestations)
	assert.Nil(t, result)
}

func TestFindAttestationEmptyList(t *testing.T) {
	result := FindAttestation("attestation1", []*pb.AttestationResult{})
	assert.Nil(t, result)
}

func TestFindAttestationFirstMatch(t *testing.T) {
	attestations := []*pb.AttestationResult{
		{Name: "attestation1"},
		{Name: "attestation1"}, // Duplicate
	}

	// Should return the first match
	result := FindAttestation("attestation1", attestations)
	assert.NotNil(t, result)
	assert.Equal(t, "attestation1", result.Name)
	assert.Equal(t, attestations[0], result)
}

func TestFindAttestationEmptyName(t *testing.T) {
	attestations := []*pb.AttestationResult{
		{Name: ""},
		{Name: "attestation1"},
	}

	result := FindAttestation("", attestations)
	assert.NotNil(t, result)
	assert.Equal(t, "", result.Name)
}

func TestFindAttestationCaseSensitive(t *testing.T) {
	attestations := []*pb.AttestationResult{
		{Name: "Attestation1"},
		{Name: "attestation2"},
	}

	result := FindAttestation("attestation1", attestations)
	assert.Nil(t, result, "should be case-sensitive")
}

func TestFindAttestationLargeList(t *testing.T) {
	attestations := make([]*pb.AttestationResult, 1000)
	for i := 0; i < 1000; i++ {
		name := "attestation"
		if i == 500 {
			name = "target_attestation"
		}
		attestations[i] = &pb.AttestationResult{Name: name}
	}

	result := FindAttestation("target_attestation", attestations)
	assert.NotNil(t, result)
	assert.Equal(t, "target_attestation", result.Name)
}
