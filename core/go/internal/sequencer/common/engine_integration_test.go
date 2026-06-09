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

package common

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type stubHooks struct {
	blockHeight int64
	nodeName    string
}

func (s *stubHooks) GetBlockHeight() int64 { return s.blockHeight }
func (s *stubHooks) GetNodeName() string   { return s.nodeName }

type engineIntegrationMocks struct {
	ei                  EngineIntegration
	allComponents       *componentsmocks.AllComponents
	domainSmartContract *componentsmocks.DomainSmartContract
	stateMgr            *componentsmocks.StateManager
	identityResolver    *componentsmocks.IdentityResolver
	txManager           *componentsmocks.TXManager
	persistence         *persistencemocks.Persistence
	domain              *componentsmocks.Domain
	contractAddress     pldtypes.EthAddress
	domainName          string
}

func newEngineIntegrationMocks(t *testing.T) *engineIntegrationMocks {
	m := &engineIntegrationMocks{
		allComponents:       componentsmocks.NewAllComponents(t),
		domainSmartContract: componentsmocks.NewDomainSmartContract(t),
		stateMgr:            componentsmocks.NewStateManager(t),
		identityResolver:    componentsmocks.NewIdentityResolver(t),
		txManager:           componentsmocks.NewTXManager(t),
		persistence:         persistencemocks.NewPersistence(t),
		domain:              componentsmocks.NewDomain(t),
		contractAddress:     *pldtypes.RandAddress(),
		domainName:          "test-domain",
	}

	m.allComponents.EXPECT().StateManager().Return(m.stateMgr).Maybe()
	m.allComponents.EXPECT().IdentityResolver().Return(m.identityResolver).Maybe()
	m.allComponents.EXPECT().TxManager().Return(m.txManager).Maybe()
	m.allComponents.EXPECT().Persistence().Return(m.persistence).Maybe()

	m.domainSmartContract.EXPECT().Domain().Return(m.domain).Maybe()
	m.domainSmartContract.EXPECT().Address().Return(m.contractAddress).Maybe()
	m.domain.EXPECT().Name().Return(m.domainName).Maybe()

	hooks := &stubHooks{blockHeight: 100, nodeName: "node1"}
	unusedDomainCtx := componentsmocks.NewDomainContext(t)
	m.ei = NewEngineIntegration(context.Background(), m.allComponents, "node1", m.domainSmartContract, unusedDomainCtx, hooks)
	return m
}

// TestAssembleAndSign_DoesNotMutatePreAssembly_SuccessPath verifies that a successful AssembleAndSign
// call does not mutate preAssembly and delivers resolved verifiers via PostAssembly.ResolvedVerifiers.
func TestAssembleAndSign_DoesNotMutatePreAssembly_SuccessPath(t *testing.T) {
	ctx := context.Background()
	m := newEngineIntegrationMocks(t)

	txID := uuid.New()
	preAssembly := &components.TransactionPreAssembly{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       "alice@node1",
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}

	freshDomainCtx := componentsmocks.NewDomainContext(t)
	m.stateMgr.EXPECT().NewDomainContext(mock.Anything, m.domain, m.contractAddress).Return(freshDomainCtx)
	freshDomainCtx.EXPECT().ImportSnapshot(mock.Anything, mock.Anything).Return(nil)
	freshDomainCtx.EXPECT().Close()

	resolvedVerifierStr := pldtypes.RandAddress().String()
	m.identityResolver.EXPECT().
		ResolveVerifier(mock.Anything, "alice@node1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).
		Return(resolvedVerifierStr, nil)

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: m.domainName,
				To:     &m.contractAddress,
			},
		},
	}
	m.txManager.EXPECT().GetResolvedTransactionByID(mock.Anything, txID).Return(localTx, nil)
	m.persistence.EXPECT().NOTX().Return(nil)

	m.domainSmartContract.EXPECT().
		AssembleTransaction(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ components.DomainContext, _ persistence.DBTX, ptx *components.PrivateTransaction, _ *components.ResolvedTransaction, _ []*prototk.ResolvedVerifier) {
			ptx.PostAssembly = &components.TransactionPostAssembly{
				AssemblyResult:  prototk.AssembleTransactionResponse_OK,
				AttestationPlan: []*prototk.AttestationRequest{},
			}
		}).
		Return(nil)

	beforeJSON, err := json.Marshal(preAssembly)
	require.NoError(t, err)

	postAssembly, err := m.ei.AssembleAndSign(ctx, txID, preAssembly, []byte("[]"), 100)

	require.NoError(t, err)
	require.NotNil(t, postAssembly)

	afterJSON, err := json.Marshal(preAssembly)
	require.NoError(t, err)
	assert.JSONEq(t, string(beforeJSON), string(afterJSON))
	require.Len(t, postAssembly.ResolvedVerifiers, 1)
	assert.Equal(t, "alice@node1", postAssembly.ResolvedVerifiers[0].Lookup)
	assert.Equal(t, resolvedVerifierStr, postAssembly.ResolvedVerifiers[0].Verifier)
}

// TestAssembleAndSign_DoesNotMutatePreAssembly_ResolverError verifies that when the identity resolver
// fails, preAssembly is not mutated.
func TestAssembleAndSign_DoesNotMutatePreAssembly_ResolverError(t *testing.T) {
	ctx := context.Background()
	m := newEngineIntegrationMocks(t)

	preAssembly := &components.TransactionPreAssembly{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       "bob@node2",
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}

	freshDomainCtx := componentsmocks.NewDomainContext(t)
	m.stateMgr.EXPECT().NewDomainContext(mock.Anything, m.domain, m.contractAddress).Return(freshDomainCtx)
	freshDomainCtx.EXPECT().ImportSnapshot(mock.Anything, mock.Anything).Return(nil)
	freshDomainCtx.EXPECT().Close()

	m.identityResolver.EXPECT().
		ResolveVerifier(mock.Anything, "bob@node2", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).
		Return("", errors.New("resolver unavailable"))

	beforeJSON, err := json.Marshal(preAssembly)
	require.NoError(t, err)

	_, err = m.ei.AssembleAndSign(ctx, uuid.New(), preAssembly, []byte("[]"), 100)
	assert.Error(t, err)

	afterJSON, marshalErr := json.Marshal(preAssembly)
	require.NoError(t, marshalErr)
	assert.JSONEq(t, string(beforeJSON), string(afterJSON))
}
