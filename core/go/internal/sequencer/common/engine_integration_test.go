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
	"fmt"
	"testing"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence/mockpersistence"
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

type eiMocks struct {
	allComponents       *componentsmocks.AllComponents
	domainSmartContract *componentsmocks.DomainSmartContract
	domainStateWriter   *componentsmocks.DomainStateWriter
	domain              *componentsmocks.Domain
	stateManager        *componentsmocks.StateManager
	txManager           *componentsmocks.TXManager
	identityResolver    *componentsmocks.IdentityResolver
	keyManager          *componentsmocks.KeyManager
	domainManager       *componentsmocks.DomainManager
}

func newTestEngineIntegration(t *testing.T) (EngineIntegration, *eiMocks) {
	t.Helper()
	m := &eiMocks{
		allComponents:       componentsmocks.NewAllComponents(t),
		domainSmartContract: componentsmocks.NewDomainSmartContract(t),
		domainStateWriter:   componentsmocks.NewDomainStateWriter(t),
		domain:              componentsmocks.NewDomain(t),
		stateManager:        componentsmocks.NewStateManager(t),
		txManager:           componentsmocks.NewTXManager(t),
		identityResolver:    componentsmocks.NewIdentityResolver(t),
		keyManager:          componentsmocks.NewKeyManager(t),
		domainManager:       componentsmocks.NewDomainManager(t),
	}

	m.allComponents.On("StateManager").Return(m.stateManager).Maybe()
	m.allComponents.On("TxManager").Return(m.txManager).Maybe()
	m.allComponents.On("IdentityResolver").Return(m.identityResolver).Maybe()
	m.allComponents.On("KeyManager").Return(m.keyManager).Maybe()
	m.allComponents.On("DomainManager").Return(m.domainManager).Maybe()

	ei := NewEngineIntegration(context.Background(), m.allComponents, "node1", m.domainSmartContract, m.domainStateWriter)
	return ei, m
}

// ─── NewEngineIntegration ─────────────────────────────────────────────

func TestNewEngineIntegration(t *testing.T) {
	ei, _ := newTestEngineIntegration(t)
	assert.NotNil(t, ei)
}

// ─── MapPotentialStates ───────────────────────────────────────────────

func TestEngineIntegration_MapPotentialStates(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	potentialStates := []*prototk.NewState{{SchemaId: "schema1"}}
	tx := &components.PrivateTransaction{ID: uuid.New()}
	expected := []*components.StateUpsert{{}}

	m.domainSmartContract.On("MapPotentialStates", mock.Anything, potentialStates, true, tx).
		Return(expected, nil).Once()

	result, err := ei.MapPotentialStates(ctx, potentialStates, tx)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

// ─── WriteStatesForTransaction ────────────────────────────────────────

func TestEngineIntegration_WriteStatesForTransaction_NoPotentialStates(t *testing.T) {
	// OutputStatesPotential == nil → no-op, returns nil without calling WritePotentialStates.
	ctx := context.Background()
	ei, _ := newTestEngineIntegration(t)

	txn := &components.PrivateTransaction{
		PostAssembly: &components.TransactionPostAssembly{},
	}
	err := ei.WriteStatesForTransaction(ctx, txn)
	require.NoError(t, err)
}

func TestEngineIntegration_WriteStatesForTransaction_WithPotentialStates_Success(t *testing.T) {
	// OutputStatesPotential != nil && OutputStates == nil → calls WritePotentialStates.
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P).Once()

	txn := &components.PrivateTransaction{
		PostAssembly: &components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				OutputStatesPotential: []*prototk.NewState{{}},
			},
		},
	}

	m.domainSmartContract.On("WritePotentialStates", mock.Anything, m.domainStateWriter, mock.Anything, txn).
		Return(nil).Once()
	m.domainSmartContract.On("Domain").Return(m.domain).Once()
	m.domain.On("Name").Return("test-domain").Once()

	err = ei.WriteStatesForTransaction(ctx, txn)
	require.NoError(t, err)
}

func TestEngineIntegration_WriteStatesForTransaction_WithPotentialStates_Error(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P).Once()

	txn := &components.PrivateTransaction{
		PostAssembly: &components.TransactionPostAssembly{
			AssembleResponse: &prototk.TransactionPostAssembly{
				InfoStatesPotential: []*prototk.NewState{{}},
			},
		},
	}

	m.domainSmartContract.On("WritePotentialStates", mock.Anything, m.domainStateWriter, mock.Anything, txn).
		Return(fmt.Errorf("write failed")).Once()

	err = ei.WriteStatesForTransaction(ctx, txn)
	require.ErrorContains(t, err, "write failed")
}

// ─── GetBlockHeight ───────────────────────────────────────────────────

func TestEngineIntegration_GetBlockHeight(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	m.domainSmartContract.On("Domain").Return(m.domain).Once()
	m.domain.On("GetBlockHeight").Return(int64(100)).Once()

	bh := ei.GetBlockHeight(ctx)
	assert.Equal(t, int64(100), bh)
}

// ─── Domain ───────────────────────────────────────────────────────────

func TestEngineIntegration_Domain(t *testing.T) {
	ei, m := newTestEngineIntegration(t)

	m.domainSmartContract.On("Domain").Return(m.domain).Once()

	result := ei.Domain()
	assert.Equal(t, m.domain, result)
}

// ─── CheckPendingPrivateStateData ─────────────────────────────────────────────

func TestEngineIntegration_CheckPendingPrivateStateData_DomainNotOptedIn(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	m.domainSmartContract.On("Domain").Return(m.domain).Once()
	m.domain.On("FullStateAvailablityRequired").Return(false).Once()

	complete, err := ei.CheckPendingPrivateStateData(ctx, 100)
	require.NoError(t, err)
	assert.True(t, complete)
}

func TestEngineIntegration_CheckPendingPrivateStateData_DomainOptedIn(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	contractAddr := *pldtypes.RandAddress()

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P).Once()

	m.domainSmartContract.On("Domain").Return(m.domain).Once()
	m.domain.On("FullStateAvailablityRequired").Return(true).Once()
	m.domainSmartContract.On("Address").Return(contractAddr).Once()
	m.stateManager.On("CheckPendingPrivateStateDataForContract", ctx, mock.Anything, contractAddr.String(), int64(100)).
		Return(true, nil).Once()

	complete, err := ei.CheckPendingPrivateStateData(ctx, 100)
	require.NoError(t, err)
	assert.True(t, complete)
}

// ─── Assemble ──────────────────────────────────────────────────

// TestAssemble_UsesStoredVerifiers verifies that when the verifiers have been resolved
// before delegation and passed in, Assemble consumes them directly — with zero identity resolver
// calls — and delivers them via PostAssembly.ResolvedVerifiers without mutating preAssembly.
func TestAssemble_UsesStoredVerifiers(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	contractAddr := *pldtypes.RandAddress()
	domainName := "test-domain"

	resolvedVerifierStr := pldtypes.RandAddress().String()
	preAssembly := &prototk.TransactionPreAssembly{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       "alice@node1",
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}
	resolvedVerifiers := []*prototk.ResolvedVerifier{
		{
			Lookup:       "alice@node1",
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
			Verifier:     resolvedVerifierStr,
		},
	}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P)

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(contractAddr)
	m.domain.On("Name").Return(domainName)

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, contractAddr).
		Return(mockDqc).Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).Return(nil).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()

	// No identity resolver expectation: the golden path must not resolve at assembly time.

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: domainName,
				To:     &contractAddr,
			},
		},
	}
	m.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&prototk.TransactionPostAssembly{
			AssemblyResult:  prototk.AssembleTransactionResponse_OK,
			AttestationPlan: []*prototk.AttestationRequest{},
		}, nil).Once()

	beforeJSON, err := json.Marshal(preAssembly)
	require.NoError(t, err)

	postAssembly, err := ei.Assemble(ctx, txID, preAssembly, resolvedVerifiers, &prototk.StateSnapshot{}, 100, localTx)

	require.NoError(t, err)
	require.NotNil(t, postAssembly)

	afterJSON, err := json.Marshal(preAssembly)
	require.NoError(t, err)
	assert.JSONEq(t, string(beforeJSON), string(afterJSON), "preAssembly must not be mutated")
	require.Len(t, postAssembly.GetResolvedVerifiers(), 1)
	assert.Equal(t, "alice@node1", postAssembly.GetResolvedVerifiers()[0].Lookup)
	assert.Equal(t, resolvedVerifierStr, postAssembly.GetResolvedVerifiers()[0].Verifier)
}

// TestAssemble_UsesSuppliedLocalTx verifies that when the resolved transaction is passed in
// (already held in memory on the originator), Assemble consumes it directly and performs no
// GetResolvedTransactionByID database read.
func TestAssemble_UsesSuppliedLocalTx(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	contractAddr := *pldtypes.RandAddress()
	domainName := "test-domain"
	preAssembly := &prototk.TransactionPreAssembly{}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P)

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(contractAddr)
	m.domain.On("Name").Return(domainName)

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, contractAddr).
		Return(mockDqc).Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).Return(nil).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			ID: &txID,
			TransactionBase: pldapi.TransactionBase{
				Domain: domainName,
				To:     &contractAddr,
			},
		},
	}

	m.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&prototk.TransactionPostAssembly{
			AssemblyResult:  prototk.AssembleTransactionResponse_OK,
			AttestationPlan: []*prototk.AttestationRequest{},
		}, nil).Once()

	postAssembly, err := ei.Assemble(ctx, txID, preAssembly, nil, &prototk.StateSnapshot{}, 100, localTx)
	require.NoError(t, err)
	require.NotNil(t, postAssembly)

	m.txManager.AssertNotCalled(t, "GetResolvedTransactionByID", mock.Anything, mock.Anything)
}

func TestEngineIntegration_Assemble_ImportSnapshotError(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	preAssembly := &prototk.TransactionPreAssembly{}

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(*pldtypes.RandAddress())
	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, mock.Anything).
		Return(mockDqc).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).
		Return(fmt.Errorf("snapshot error")).Once()

	_, err := ei.Assemble(ctx, txID, preAssembly, nil, &prototk.StateSnapshot{}, 100, nil)
	require.ErrorContains(t, err, "snapshot error")
}

// ─── ResolveVerifiers ─────────────────────────────────────────────────

func TestEngineIntegration_ResolveVerifiers_Empty(t *testing.T) {
	ctx := context.Background()
	ei, _ := newTestEngineIntegration(t)
	// No resolver expectation: an empty request list must not touch the identity resolver.
	resolved, err := ei.ResolveVerifiers(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, resolved)
}

func TestEngineIntegration_ResolveVerifiers_ConcurrentSuccessInOrder(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	required := []*prototk.ResolveVerifierRequest{
		{Lookup: "alice@node1", Algorithm: "algo1", VerifierType: "type1"},
		{Lookup: "bob@node2", Algorithm: "algo2", VerifierType: "type2"},
	}

	m.identityResolver.On("ResolveVerifierAsync", mock.Anything, "alice@node1", "algo1", "type1", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			args.Get(4).(func(context.Context, string))(ctx, "verifier-alice")
		}).Return().Once()
	m.identityResolver.On("ResolveVerifierAsync", mock.Anything, "bob@node2", "algo2", "type2", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			args.Get(4).(func(context.Context, string))(ctx, "verifier-bob")
		}).Return().Once()

	resolved, err := ei.ResolveVerifiers(ctx, required)
	require.NoError(t, err)
	require.Len(t, resolved, 2)
	// Results are returned in request order regardless of resolution completion order.
	assert.Equal(t, "alice@node1", resolved[0].Lookup)
	assert.Equal(t, "verifier-alice", resolved[0].Verifier)
	assert.Equal(t, "bob@node2", resolved[1].Lookup)
	assert.Equal(t, "verifier-bob", resolved[1].Verifier)
}

func TestEngineIntegration_ResolveVerifiers_FirstErrorReturned(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	required := []*prototk.ResolveVerifierRequest{
		{Lookup: "alice@node1", Algorithm: "algo1", VerifierType: "type1"},
		{Lookup: "bob@node2", Algorithm: "algo2", VerifierType: "type2"},
	}

	m.identityResolver.On("ResolveVerifierAsync", mock.Anything, "alice@node1", "algo1", "type1", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			args.Get(4).(func(context.Context, string))(ctx, "verifier-alice")
		}).Return().Once()
	m.identityResolver.On("ResolveVerifierAsync", mock.Anything, "bob@node2", "algo2", "type2", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			args.Get(5).(func(context.Context, error))(ctx, errors.New("bob offline"))
		}).Return().Once()

	resolved, err := ei.ResolveVerifiers(ctx, required)
	require.ErrorContains(t, err, "bob offline")
	assert.Nil(t, resolved)
}

func TestEngineIntegration_Assemble_WrongDomain(t *testing.T) {
	// Transaction exists but is for a different domain → logs error and returns.
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	contractAddr := *pldtypes.RandAddress()
	preAssembly := &prototk.TransactionPreAssembly{}

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(contractAddr)
	m.domain.On("Name").Return("domain1")

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, mock.Anything).
		Return(mockDqc).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).Return(nil).Once()

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: "other-domain",
				To:     &contractAddr,
			},
		},
	}

	_, err := ei.Assemble(ctx, txID, preAssembly, nil, nil, 100, localTx)
	require.Error(t, err)
}

func TestEngineIntegration_Assemble_AssembleTransactionError(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	contractAddr := *pldtypes.RandAddress()
	preAssembly := &prototk.TransactionPreAssembly{}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P)

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(contractAddr)
	m.domain.On("Name").Return("domain1")

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, mock.Anything).
		Return(mockDqc).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).Return(nil).Once()

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Domain: "domain1",
				To:     &contractAddr,
			},
		},
	}

	m.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("assemble failed")).Once()

	_, err = ei.Assemble(ctx, txID, preAssembly, nil, nil, 100, localTx)
	require.ErrorContains(t, err, "assemble failed")
}

func TestEngineIntegration_Assemble_NilPostAssembly(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	contractAddr := *pldtypes.RandAddress()
	preAssembly := &prototk.TransactionPreAssembly{}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P)

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(contractAddr)
	m.domain.On("Name").Return("domain1")

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, mock.Anything).
		Return(mockDqc).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).Return(nil).Once()

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{Domain: "domain1", To: &contractAddr},
		},
	}

	// AssembleTransaction returns nil PostAssembly (no error) — treated as internal error.
	m.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, nil).Once()

	_, err = ei.Assemble(ctx, txID, preAssembly, nil, nil, 100, localTx)
	require.Error(t, err)
}

func TestEngineIntegration_Assemble_UnsupportedAttestationType(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	contractAddr := *pldtypes.RandAddress()
	preAssembly := &prototk.TransactionPreAssembly{}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P)

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(contractAddr)
	m.domain.On("Name").Return("domain1")

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, mock.Anything).
		Return(mockDqc).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).Return(nil).Once()

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{Domain: "domain1", To: &contractAddr},
		},
	}

	m.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&prototk.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{AttestationType: prototk.AttestationType(99)}, // unsupported type
			},
		}, nil).Once()

	_, err = ei.Assemble(ctx, txID, preAssembly, nil, nil, 100, localTx)
	require.Error(t, err)
}

func TestEngineIntegration_SignAttestation_LocalParty(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	attRequest := &prototk.AttestationRequest{
		Name:            "sig",
		AttestationType: prototk.AttestationType_SIGN,
		Algorithm:       "ecdsa",
		VerifierType:    "eth_address",
		Payload:         []byte("payload"),
		PayloadType:     "bytes",
	}

	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{Verifier: "0xabc"},
	}
	m.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, "alice", "ecdsa", "eth_address").
		Return(resolvedKey, nil).Once()
	m.keyManager.On("Sign", mock.Anything, resolvedKey, "bytes", []byte("payload")).
		Return([]byte("signature"), nil).Once()

	result, err := ei.SignAttestation(ctx, uuid.New(), attRequest, "alice@node1")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "sig", result.Name)
	assert.Equal(t, prototk.AttestationType_SIGN, result.AttestationType)
	assert.Equal(t, []byte("signature"), result.Payload)
	assert.Equal(t, "alice@node1", result.Verifier.Lookup)
	assert.Equal(t, "0xabc", result.Verifier.Verifier)
}

func TestEngineIntegration_SignAttestation_RemoteParty(t *testing.T) {
	// Party is on a different node — SignAttestation returns (nil, nil), it is not signed locally.
	ctx := context.Background()
	ei, _ := newTestEngineIntegration(t)

	attRequest := &prototk.AttestationRequest{AttestationType: prototk.AttestationType_SIGN}

	result, err := ei.SignAttestation(ctx, uuid.New(), attRequest, "bob@node2")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestEngineIntegration_Assemble_EndorseAttestationType(t *testing.T) {
	// ENDORSE attestation type is ignored (handled later) — no error.
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	contractAddr := *pldtypes.RandAddress()
	preAssembly := &prototk.TransactionPreAssembly{}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P)

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(contractAddr)
	m.domain.On("Name").Return("domain1")

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, mock.Anything).
		Return(mockDqc).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).Return(nil).Once()

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{Domain: "domain1", To: &contractAddr},
		},
	}

	m.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AttestationPlan: []*prototk.AttestationRequest{
				{AttestationType: prototk.AttestationType_ENDORSE},
			},
		}, nil).Once()

	result, err := ei.Assemble(ctx, txID, preAssembly, nil, nil, 100, localTx)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEngineIntegration_SignAttestation_ResolveKeyError(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	attRequest := &prototk.AttestationRequest{
		AttestationType: prototk.AttestationType_SIGN,
		Algorithm:       "ecdsa",
		VerifierType:    "eth_address",
	}

	m.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, "alice", "ecdsa", "eth_address").
		Return(nil, fmt.Errorf("key error")).Once()

	_, err := ei.SignAttestation(ctx, uuid.New(), attRequest, "alice@node1")
	require.ErrorContains(t, err, "key error")
}

func TestEngineIntegration_SignAttestation_InvalidPartyLocator(t *testing.T) {
	// Party name with two "@" separators → 3 parts → Validate returns an error.
	ctx := context.Background()
	ei, _ := newTestEngineIntegration(t)

	attRequest := &prototk.AttestationRequest{
		AttestationType: prototk.AttestationType_SIGN,
		Algorithm:       "ecdsa",
		VerifierType:    "eth_address",
	}

	_, err := ei.SignAttestation(ctx, uuid.New(), attRequest, "me@node1@extra")
	require.Error(t, err)
}

func TestEngineIntegration_Assemble_DebugLogging(t *testing.T) {
	// Enable debug logging so the log.IsDebugEnabled() branch is taken.
	log.SetLevel("debug")
	defer log.SetLevel("info")

	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	contractAddr := *pldtypes.RandAddress()
	preAssembly := &prototk.TransactionPreAssembly{}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P)

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(contractAddr)
	m.domain.On("Name").Return("domain1")

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, mock.Anything).
		Return(mockDqc).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).Return(nil).Once()

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{Domain: "domain1", To: &contractAddr},
		},
	}

	m.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&prototk.TransactionPostAssembly{
			AssemblyResult:  prototk.AssembleTransactionResponse_OK,
			AttestationPlan: []*prototk.AttestationRequest{},
		}, nil).Once()

	result, err := ei.Assemble(ctx, txID, preAssembly, nil, nil, 100, localTx)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestEngineIntegration_SignAttestation_SignError(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	attRequest := &prototk.AttestationRequest{
		AttestationType: prototk.AttestationType_SIGN,
		Algorithm:       "ecdsa",
		VerifierType:    "eth_address",
		Payload:         []byte("data"),
		PayloadType:     "bytes",
	}

	resolvedKey := &pldapi.KeyMappingAndVerifier{
		Verifier: &pldapi.KeyVerifier{Verifier: "0xabc"},
	}
	m.keyManager.On("ResolveKeyNewDatabaseTX", mock.Anything, "alice", "ecdsa", "eth_address").
		Return(resolvedKey, nil).Once()
	m.keyManager.On("Sign", mock.Anything, resolvedKey, "bytes", []byte("data")).
		Return(nil, fmt.Errorf("sign error")).Once()

	_, err := ei.SignAttestation(ctx, uuid.New(), attRequest, "alice@node1")
	require.ErrorContains(t, err, "sign error")
}

// TestEngineIntegration_Assemble_DoesNotSign verifies the assemble path no longer signs: a plan with a
// local SIGN attestation returns empty Signatures and makes zero KeyManager calls (KeyManager is not
// expected in the mocks, so any sign call would fail the test).
func TestEngineIntegration_Assemble_DoesNotSign(t *testing.T) {
	ctx := context.Background()
	ei, m := newTestEngineIntegration(t)

	txID := uuid.New()
	contractAddr := *pldtypes.RandAddress()
	preAssembly := &prototk.TransactionPreAssembly{}

	mp, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	m.allComponents.On("Persistence").Return(mp.P)

	m.domainSmartContract.On("Domain").Return(m.domain)
	m.domainSmartContract.On("Address").Return(contractAddr)
	m.domain.On("Name").Return("domain1")

	mockDqc := componentsmocks.NewDomainQueryContext(t)
	m.stateManager.On("NewDomainQueryContext", mock.Anything, m.domain, mock.Anything).
		Return(mockDqc).Once()
	mockDqc.On("Close", mock.Anything).Return().Once()
	mockDqc.On("ImportSnapshot", mock.Anything, mock.Anything).Return(nil).Once()

	localTx := &components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{Domain: "domain1", To: &contractAddr},
		},
	}

	m.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&prototk.TransactionPostAssembly{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
			AttestationPlan: []*prototk.AttestationRequest{
				{
					Name:            "sig",
					AttestationType: prototk.AttestationType_SIGN,
					Parties:         []string{"alice@node1"},
				},
			},
		}, nil).Once()

	result, err := ei.Assemble(ctx, txID, preAssembly, nil, nil, 100, localTx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.GetSignatures())
}
