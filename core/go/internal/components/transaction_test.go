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

package components

import (
	"testing"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestReleasePostAssemblyData(t *testing.T) {
	pt := &PrivateTransaction{
		ID:      uuid.New(),
		Domain:  "test-domain",
		Address: *pldtypes.RandAddress(),
		Signer:  "signer@node1",
		PreAssembly: &TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{},
		},
		PostAssembly: &TransactionPostAssembly{
			OutputStates: []*FullState{{Data: pldtypes.RawJSON(`{}`)}},
			InputStates:  []*FullState{{Data: pldtypes.RawJSON(`{}`)}},
		},
		PreparedPublicTransaction:  &pldapi.TransactionInput{},
		PreparedPrivateTransaction: &pldapi.TransactionInput{},
		PreparedMetadata:           pldtypes.RawJSON(`{"meta":true}`),
	}

	savedID := pt.ID
	savedDomain := pt.Domain
	savedAddress := pt.Address
	savedSigner := pt.Signer

	pt.CleanUpPostAssemblyData()

	assert.Nil(t, pt.PostAssembly)
	assert.Nil(t, pt.PreparedPublicTransaction)
	assert.Nil(t, pt.PreparedPrivateTransaction)
	assert.Nil(t, pt.PreparedMetadata)

	assert.NotNil(t, pt.PreAssembly, "PreAssembly should be preserved")
	assert.Equal(t, savedID, pt.ID)
	assert.Equal(t, savedDomain, pt.Domain)
	assert.Equal(t, savedAddress, pt.Address)
	assert.Equal(t, savedSigner, pt.Signer)
}

func TestReleasePostAssemblyData_NilFields(t *testing.T) {
	pt := &PrivateTransaction{ID: uuid.New()}
	pt.CleanUpPostAssemblyData()

	assert.Nil(t, pt.PostAssembly)
	assert.Nil(t, pt.PreparedPublicTransaction)
}
