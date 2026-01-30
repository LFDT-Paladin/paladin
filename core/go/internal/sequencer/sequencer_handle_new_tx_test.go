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

package sequencer

import (
	"context"
	"errors"
	"testing"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestHandleNewTx_Deploy_SubmitModeNotAuto verifies that when tx.To is nil (deploy) and SubmitMode is not Auto,
// HandleNewTx returns MsgSequencerPrepareNotSupportedDeploy.
func TestHandleNewTx_Deploy_SubmitModeNotAuto(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeExternal.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     nil, // deploy
				},
			},
		},
	}

	dbTX := persistencemocks.NewDBTX(t)
	dbTX.EXPECT().AddPostCommit(mock.Anything).Maybe().Unset()

	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.Error(t, err)
	assert.Regexp(t, "not supported|Prepare|deploy", err.Error())
}

// TestHandleNewTx_Deploy_SubmitModeAuto_Success verifies that when tx.To is nil (deploy) and SubmitMode is Auto,
// HandleNewTx calls handleDeployTx which gets the domain and starts the deployment loop; we verify HandleNewTx returns nil.
func TestHandleNewTx_Deploy_SubmitModeAuto_Success(t *testing.T) {
	ctx := context.Background()
	dm := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, dm)

	dm.components.EXPECT().DomainManager().Return(dm.domainManager).Once()
	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().InitDeploy(ctx, mock.Anything).Return(nil).Once()
	// deploymentLoop runs in a goroutine; PrepareDeploy fails so revertDeploy runs
	domain.EXPECT().PrepareDeploy(mock.Anything, mock.Anything).Return(errors.New("test done")).Maybe()
	dm.domainManager.EXPECT().GetDomainByName(ctx, "test-domain").Return(domain, nil).Once()
	dm.metrics.EXPECT().IncDispatchedTransactions().Maybe()
	dm.syncPoints.EXPECT().
		QueueTransactionFinalize(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Maybe()

	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     nil, // deploy
				},
			},
		},
	}

	dbTX := persistencemocks.NewDBTX(t)
	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.NoError(t, err)
	domain.AssertExpectations(t)
}

// TestHandleNewTx_Deploy_DomainNotFound verifies that when tx.To is nil (deploy), SubmitMode is Auto,
// but GetDomainByName returns an error, HandleNewTx returns that error.
func TestHandleNewTx_Deploy_DomainNotFound(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetDomainByName(ctx, "test-domain").Return(nil, errors.New("domain not found")).Once()

	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     nil,
				},
			},
		},
	}

	dbTX := persistencemocks.NewDBTX(t)
	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.Error(t, err)
	assert.Regexp(t, "domain not found|Domain", err.Error())
}

// TestHandleNewTx_FunctionNotProvided verifies that when tx.To is set (regular tx) but Function is nil,
// HandleNewTx returns MsgSequencerFunctionNotProvided.
func TestHandleNewTx_FunctionNotProvided(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	toAddr := pldtypes.RandAddress()
	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     toAddr,
				},
			},
			Function: nil, // not provided
		},
	}

	dbTX := persistencemocks.NewDBTX(t)
	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.Error(t, err)
	assert.Regexp(t, "Function|not provided", err.Error())
}

// TestHandleNewTx_FunctionDefinitionNil verifies that when tx.To is set and Function is set but Definition is nil,
// HandleNewTx returns MsgSequencerFunctionNotProvided.
func TestHandleNewTx_FunctionDefinitionNil(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	toAddr := pldtypes.RandAddress()
	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     toAddr,
				},
			},
			Function: &components.ResolvedFunction{
				Definition: nil, // definition not provided
				Signature:  "foo()",
			},
		},
	}

	dbTX := persistencemocks.NewDBTX(t)
	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.Error(t, err)
	assert.Regexp(t, "Function|not provided", err.Error())
}

// TestHandleNewTx_RegularTx_Success verifies the full path when tx.To is set, Function and Definition are set:
// GetSmartContractByAddress, InitTransaction (sets PreAssembly), LoadSequencer (returns pre-stored sequencer), AddPostCommit.
func TestHandleNewTx_RegularTx_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	contractAddr := pldtypes.RandAddress()
	// Pre-store a sequencer so LoadSequencer returns it without creating a new one
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	domainAPI := componentsmocks.NewDomainSmartContract(t)
	domainMock := componentsmocks.NewDomain(t)
	domainAPI.EXPECT().Domain().Return(domainMock).Once()
	domainMock.EXPECT().Name().Return("test-domain").Once()
	domainAPI.EXPECT().InitTransaction(ctx, mock.Anything, mock.Anything).
		Run(func(_ context.Context, ptx *components.PrivateTransaction, _ *components.ResolvedTransaction) {
			ptx.PreAssembly = &components.TransactionPreAssembly{}
		}).Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(domainAPI, nil).Once()

	// Existing sequencer path: GetCurrentCoordinator returns non-empty so setInitialCoordinator is not called
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Maybe()

	dbTX := persistencemocks.NewDBTX(t)
	dbTX.EXPECT().AddPostCommit(mock.Anything).Once()
	dbTX.EXPECT().FullTransaction().Return(true).Maybe()

	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     contractAddr,
				},
			},
			Function: &components.ResolvedFunction{
				Definition: &abi.Entry{Type: abi.Function, Name: "doSomething", Inputs: abi.ParameterArray{}},
				Signature:  "doSomething()",
			},
		},
	}

	err := sm.HandleNewTx(ctx, dbTX, txi)
	require.NoError(t, err)
	domainAPI.AssertExpectations(t)
	dbTX.AssertExpectations(t)
}

// ---- HandleTxResume tests ----
// HandleTxResume is like HandleNewTx but takes no dbTX and calls handleTx with NOTX() and resume=true.

// TestHandleTxResume_Deploy_SubmitModeNotAuto verifies that when tx.To is nil (deploy) and SubmitMode is not Auto,
// HandleTxResume returns MsgSequencerPrepareNotSupportedDeploy.
func TestHandleTxResume_Deploy_SubmitModeNotAuto(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeExternal.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     nil, // deploy
				},
			},
		},
	}

	err := sm.HandleTxResume(ctx, txi)
	require.Error(t, err)
	assert.Regexp(t, "not supported|Prepare|deploy", err.Error())
}

// TestHandleTxResume_Deploy_SubmitModeAuto_Success verifies that when tx.To is nil (deploy) and SubmitMode is Auto,
// HandleTxResume calls handleDeployTx; we verify HandleTxResume returns nil.
func TestHandleTxResume_Deploy_SubmitModeAuto_Success(t *testing.T) {
	ctx := context.Background()
	dm := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, dm)

	dm.components.EXPECT().DomainManager().Return(dm.domainManager).Once()
	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().InitDeploy(ctx, mock.Anything).Return(nil).Once()
	domain.EXPECT().PrepareDeploy(mock.Anything, mock.Anything).Return(errors.New("test done")).Maybe()
	dm.domainManager.EXPECT().GetDomainByName(ctx, "test-domain").Return(domain, nil).Once()
	dm.metrics.EXPECT().IncDispatchedTransactions().Maybe()
	dm.syncPoints.EXPECT().
		QueueTransactionFinalize(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Maybe()

	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     nil, // deploy
				},
			},
		},
	}

	err := sm.HandleTxResume(ctx, txi)
	require.NoError(t, err)
	domain.AssertExpectations(t)
}

// TestHandleTxResume_Deploy_DomainNotFound verifies that when tx.To is nil (deploy), SubmitMode is Auto,
// but GetDomainByName returns an error, HandleTxResume returns that error.
func TestHandleTxResume_Deploy_DomainNotFound(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetDomainByName(ctx, "test-domain").Return(nil, errors.New("domain not found")).Once()

	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     nil,
				},
			},
		},
	}

	err := sm.HandleTxResume(ctx, txi)
	require.Error(t, err)
	assert.Regexp(t, "domain not found|Domain", err.Error())
}

// TestHandleTxResume_FunctionNotProvided verifies that when tx.To is set (regular tx) but Function is nil,
// HandleTxResume returns MsgSequencerFunctionNotProvided.
func TestHandleTxResume_FunctionNotProvided(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	toAddr := pldtypes.RandAddress()
	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     toAddr,
				},
			},
			Function: nil,
		},
	}

	err := sm.HandleTxResume(ctx, txi)
	require.Error(t, err)
	assert.Regexp(t, "Function|not provided", err.Error())
}

// TestHandleTxResume_FunctionDefinitionNil verifies that when tx.To is set and Function is set but Definition is nil,
// HandleTxResume returns MsgSequencerFunctionNotProvided.
func TestHandleTxResume_FunctionDefinitionNil(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	toAddr := pldtypes.RandAddress()
	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     toAddr,
				},
			},
			Function: &components.ResolvedFunction{
				Definition: nil,
				Signature:  "foo()",
			},
		},
	}

	err := sm.HandleTxResume(ctx, txi)
	require.Error(t, err)
	assert.Regexp(t, "Function|not provided", err.Error())
}

// TestHandleTxResume_RegularTx_Success verifies the resume path when tx.To is set and Function/Definition are set:
// handleTx is called with NOTX() and resume=true, so QueueEvent is called directly (no AddPostCommit).
func TestHandleTxResume_RegularTx_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	contractAddr := pldtypes.RandAddress()
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = existingSeq
	sm.sequencersLock.Unlock()

	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Once()
	mocks.persistence.EXPECT().NOTX().Return(nil).Once()
	domainAPI := componentsmocks.NewDomainSmartContract(t)
	domainMock := componentsmocks.NewDomain(t)
	domainAPI.EXPECT().Domain().Return(domainMock).Once()
	domainMock.EXPECT().Name().Return("test-domain").Once()
	domainAPI.EXPECT().InitTransaction(ctx, mock.Anything, mock.Anything).
		Run(func(_ context.Context, ptx *components.PrivateTransaction, _ *components.ResolvedTransaction) {
			ptx.PreAssembly = &components.TransactionPreAssembly{}
		}).Return(nil).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, nil, *contractAddr).Return(domainAPI, nil).Once()

	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Maybe()
	mocks.originator.EXPECT().QueueEvent(ctx, mock.MatchedBy(func(ev interface{}) bool {
		_, ok := ev.(*originator.TransactionCreatedEvent)
		return ok
	})).Once()

	txID := uuid.New()
	txi := &components.ValidatedTransaction{
		ResolvedTransaction: components.ResolvedTransaction{
			Transaction: &pldapi.Transaction{
				ID:         &txID,
				SubmitMode: pldapi.SubmitModeAuto.Enum(),
				TransactionBase: pldapi.TransactionBase{
					Domain: "test-domain",
					From:   "from@test-node",
					To:     contractAddr,
				},
			},
			Function: &components.ResolvedFunction{
				Definition: &abi.Entry{Type: abi.Function, Name: "doSomething", Inputs: abi.ParameterArray{}},
				Signature:  "doSomething()",
			},
		},
	}

	err := sm.HandleTxResume(ctx, txi)
	require.NoError(t, err)
	domainAPI.AssertExpectations(t)
	mocks.originator.AssertExpectations(t)
}
