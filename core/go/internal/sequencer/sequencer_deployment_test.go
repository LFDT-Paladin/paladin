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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// deploymentTestMocks extends the lifecycle mocks with IdentityResolver and syncPoints for deploymentLoop tests.
type deploymentTestMocks struct {
	*sequencerLifecycleTestMocks
	identityResolver *componentsmocks.IdentityResolver
}

func newDeploymentTestMocks(t *testing.T) *deploymentTestMocks {
	return &deploymentTestMocks{
		sequencerLifecycleTestMocks: newSequencerLifecycleTestMocks(t),
		identityResolver:            componentsmocks.NewIdentityResolver(t),
	}
}

func newSequencerManagerForDeploymentTesting(t *testing.T, mocks *deploymentTestMocks) *sequencerManager {
	sm := newSequencerManagerForTesting(t, mocks.sequencerLifecycleTestMocks)
	sm.syncPoints = mocks.syncPoints
	// Each test sets IdentityResolver() expectation as needed
	return sm
}

// TestDeploymentLoop_VerifierResolutionFails covers the path where ResolveVerifier returns an error;
// deploymentLoop should set err, skip evaluateDeployment, log and return.
func TestDeploymentLoop_VerifierResolutionFails(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	mocks.components.EXPECT().IdentityResolver().Return(mocks.identityResolver).Once()
	mocks.identityResolver.EXPECT().
		ResolveVerifier(ctx, "verifier1@node1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).
		Return("", errors.New("resolution failed"))

	domain := componentsmocks.NewDomain(t)
	// PrepareDeploy must not be called when verifier resolution fails
	domain.EXPECT().PrepareDeploy(mock.Anything, mock.Anything).Maybe().Unset()

	tx := &components.PrivateContractDeploy{
		ID:     uuid.New(),
		Domain: "test-domain",
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{Lookup: "verifier1@node1", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
		},
	}

	sm.deploymentLoop(ctx, domain, tx)

	// Verifier resolution failed so the first verifier slot was never set (loop broke on error)
	require.Len(t, tx.Verifiers, 1)
	assert.Nil(t, tx.Verifiers[0])
	mocks.identityResolver.AssertExpectations(t)
}

// TestDeploymentLoop_VerifierResolutionSuccess_ThenEvaluateFails covers the path where verifiers are resolved
// successfully and then evaluateDeployment is invoked; PrepareDeploy returns error so revertDeploy runs.
func TestDeploymentLoop_VerifierResolutionSuccess_ThenEvaluateFails(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	mocks.components.EXPECT().IdentityResolver().Return(mocks.identityResolver).Once()
	mocks.identityResolver.EXPECT().
		ResolveVerifier(ctx, "verifier1@node1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).
		Return("0x1234567890123456789012345678901234567890", nil)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(errors.New("prepare failed")).Once()

	// When evaluateDeployment fails, revertDeploy calls QueueTransactionFinalize
	mocks.syncPoints.EXPECT().
		QueueTransactionFinalize(ctx, mock.Anything, pldtypes.EthAddress{}, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Maybe()

	tx := &components.PrivateContractDeploy{
		ID:     uuid.New(),
		Domain: "test-domain",
		From:   "from@test-node",
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{Lookup: "verifier1@node1", Algorithm: algorithms.ECDSA_SECP256K1, VerifierType: verifiers.ETH_ADDRESS},
		},
	}

	sm.deploymentLoop(ctx, domain, tx)

	// Verifiers should be populated after successful resolution
	require.Len(t, tx.Verifiers, 1)
	assert.Equal(t, "0x1234567890123456789012345678901234567890", tx.Verifiers[0].Verifier)
	domain.AssertExpectations(t)
	mocks.identityResolver.AssertExpectations(t)
}

// deployTxLocalSigner returns a deploy tx with Signer set to local node (test-node) for evaluateDeployment tests.
func deployTxLocalSigner(id uuid.UUID) *components.PrivateContractDeploy {
	return &components.PrivateContractDeploy{
		ID:     id,
		Domain: "test-domain",
		From:   "from@test-node",
		Signer: "local-id@test-node",
	}
}

// setupEvaluateDeploymentMocks sets component expectations needed for evaluateDeployment (after PrepareDeploy).
func setupEvaluateDeploymentMocks(t *testing.T, mocks *deploymentTestMocks, ctx context.Context) {
	mocks.components.EXPECT().PublicTxManager().Return(mocks.publicTxManager).Maybe()
	mocks.components.EXPECT().KeyManager().Return(mocks.keyManager).Maybe()
	mocks.components.EXPECT().Persistence().Return(mocks.persistence).Maybe()
	mocks.persistence.EXPECT().NOTX().Return(nil).Maybe()
}

// TestEvaluateDeployment_PrepareDeployFails covers the path where domain.PrepareDeploy returns an error;
// evaluateDeployment should call revertDeploy and return the wrapped error.
func TestEvaluateDeployment_PrepareDeployFails(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(errors.New("prepare failed")).Once()

	mocks.syncPoints.EXPECT().
		QueueTransactionFinalize(ctx, mock.Anything, pldtypes.EthAddress{}, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Once()

	tx := deployTxLocalSigner(uuid.New())
	err := sm.evaluateDeployment(ctx, domain, tx)
	require.Error(t, err)
	domain.AssertExpectations(t)
}

// TestEvaluateDeployment_NonLocalSigner covers the path where tx.Signer is for a different node;
// evaluateDeployment should return an error without calling revertDeploy.
func TestEvaluateDeployment_NonLocalSigner(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(nil).Once()

	setupEvaluateDeploymentMocks(t, mocks, ctx)

	tx := deployTxLocalSigner(uuid.New())
	tx.Signer = "other@other-node"
	err := sm.evaluateDeployment(ctx, domain, tx)
	require.Error(t, err)
	domain.AssertExpectations(t)
	// revertDeploy must not be called (no QueueTransactionFinalize)
	mocks.syncPoints.AssertNotCalled(t, "QueueTransactionFinalize")
}

// TestEvaluateDeployment_KeyManagerResolveFails covers the path where KeyManager.ResolveEthAddressBatchNewDatabaseTX fails;
// evaluateDeployment should call revertDeploy and return.
func TestEvaluateDeployment_KeyManagerResolveFails(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(nil).Once()

	setupEvaluateDeploymentMocks(t, mocks, ctx)
	mocks.keyManager.EXPECT().
		ResolveEthAddressBatchNewDatabaseTX(ctx, mock.MatchedBy(func(ids []string) bool { return len(ids) == 1 && ids[0] == "local-id" })).
		Return(nil, errors.New("resolve failed")).Once()

	mocks.syncPoints.EXPECT().
		QueueTransactionFinalize(ctx, mock.Anything, pldtypes.EthAddress{}, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Once()

	tx := deployTxLocalSigner(uuid.New())
	err := sm.evaluateDeployment(ctx, domain, tx)
	require.Error(t, err)
	domain.AssertExpectations(t)
}

// TestEvaluateDeployment_NeitherInvokeNorDeploy covers the path where neither InvokeTransaction nor DeployTransaction is set;
// evaluateDeployment should call revertDeploy and return.
func TestEvaluateDeployment_NeitherInvokeNorDeploy(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(nil).Once()

	setupEvaluateDeploymentMocks(t, mocks, ctx)
	addr := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().
		ResolveEthAddressBatchNewDatabaseTX(ctx, mock.Anything).
		Return([]*pldtypes.EthAddress{addr}, nil).Once()

	mocks.syncPoints.EXPECT().
		QueueTransactionFinalize(ctx, mock.Anything, pldtypes.EthAddress{}, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Once()

	tx := deployTxLocalSigner(uuid.New())
	// InvokeTransaction and DeployTransaction both nil
	err := sm.evaluateDeployment(ctx, domain, tx)
	require.Error(t, err)
	domain.AssertExpectations(t)
}

// TestEvaluateDeployment_DeployTransactionNotImplemented covers the path where DeployTransaction is set;
// evaluateDeployment should call revertDeploy with "deployTransaction not implemented".
func TestEvaluateDeployment_DeployTransactionNotImplemented(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(nil).Once()

	setupEvaluateDeploymentMocks(t, mocks, ctx)
	addr := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().
		ResolveEthAddressBatchNewDatabaseTX(ctx, mock.Anything).
		Return([]*pldtypes.EthAddress{addr}, nil).Once()

	mocks.syncPoints.EXPECT().
		QueueTransactionFinalize(ctx, mock.Anything, pldtypes.EthAddress{}, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Once()

	tx := deployTxLocalSigner(uuid.New())
	tx.DeployTransaction = &components.EthDeployTransaction{}
	err := sm.evaluateDeployment(ctx, domain, tx)
	require.Error(t, err)
	domain.AssertExpectations(t)
}

// TestEvaluateDeployment_EncodeCallDataFails covers the path where InvokeTransaction is set but EncodeCallDataCtx fails;
// evaluateDeployment should call revertDeploy and return.
func TestEvaluateDeployment_EncodeCallDataFails(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(nil).Once()

	setupEvaluateDeploymentMocks(t, mocks, ctx)
	addr := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	mocks.keyManager.EXPECT().
		ResolveEthAddressBatchNewDatabaseTX(ctx, mock.Anything).
		Return([]*pldtypes.EthAddress{addr}, nil).Once()

	mocks.syncPoints.EXPECT().
		QueueTransactionFinalize(ctx, mock.Anything, pldtypes.EthAddress{}, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Once()

	toAddr := pldtypes.RandAddress()
	tx := deployTxLocalSigner(uuid.New())
	// Function with one required input; nil ComponentValue causes EncodeCallDataCtx to fail
	tx.InvokeTransaction = &components.EthTransaction{
		FunctionABI: &abi.Entry{Type: abi.Function, Name: "deploy", Inputs: abi.ParameterArray{{Type: "uint256", Name: "x"}}},
		To:          *toAddr,
		Inputs:      nil,
	}
	err := sm.evaluateDeployment(ctx, domain, tx)
	require.Error(t, err)
	domain.AssertExpectations(t)
}

// TestEvaluateDeployment_ValidateTransactionFails covers the path where PublicTxManager.ValidateTransaction fails;
// evaluateDeployment should call revertDeploy and return.
func TestEvaluateDeployment_ValidateTransactionFails(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	fnABI := &abi.Entry{Type: abi.Function, Name: "deploy", Inputs: abi.ParameterArray{}}
	cv, err := fnABI.Inputs.ParseJSONCtx(ctx, []byte("[]"))
	require.NoError(t, err)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(nil).Once()

	setupEvaluateDeploymentMocks(t, mocks, ctx)
	addr := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().
		ResolveEthAddressBatchNewDatabaseTX(ctx, mock.Anything).
		Return([]*pldtypes.EthAddress{addr}, nil).Once()

	mocks.publicTxManager.EXPECT().
		ValidateTransaction(ctx, nil, mock.Anything).
		Return(errors.New("validation failed")).Once()

	mocks.syncPoints.EXPECT().
		QueueTransactionFinalize(ctx, mock.Anything, pldtypes.EthAddress{}, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Once()

	toAddr := pldtypes.RandAddress()
	tx := deployTxLocalSigner(uuid.New())
	tx.InvokeTransaction = &components.EthTransaction{
		FunctionABI: fnABI,
		To:          *toAddr,
		Inputs:      cv,
	}
	err = sm.evaluateDeployment(ctx, domain, tx)
	require.Error(t, err)
	domain.AssertExpectations(t)
}

// TestEvaluateDeployment_PersistDeployDispatchBatchFails covers the path where syncPoints.PersistDeployDispatchBatch fails;
// evaluateDeployment should call revertDeploy and return.
func TestEvaluateDeployment_PersistDeployDispatchBatchFails(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	fnABI := &abi.Entry{Type: abi.Function, Name: "deploy", Inputs: abi.ParameterArray{}}
	cv, err := fnABI.Inputs.ParseJSONCtx(ctx, []byte("[]"))
	require.NoError(t, err)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(nil).Once()

	setupEvaluateDeploymentMocks(t, mocks, ctx)
	addr := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().
		ResolveEthAddressBatchNewDatabaseTX(ctx, mock.Anything).
		Return([]*pldtypes.EthAddress{addr}, nil).Once()

	mocks.publicTxManager.EXPECT().
		ValidateTransaction(ctx, nil, mock.Anything).
		Return(nil).Once()

	mocks.syncPoints.EXPECT().
		PersistDeployDispatchBatch(ctx, mock.MatchedBy(func(b *syncpoints.DispatchBatch) bool {
			return b != nil && len(b.PublicDispatches) == 1 && len(b.PublicDispatches[0].PublicTxs) == 1
		})).
		Return(errors.New("persist failed")).Once()
	mocks.syncPoints.EXPECT().
		QueueTransactionFinalize(ctx, mock.Anything, pldtypes.EthAddress{}, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return().Once()

	toAddr := pldtypes.RandAddress()
	tx := deployTxLocalSigner(uuid.New())
	tx.InvokeTransaction = &components.EthTransaction{
		FunctionABI: fnABI,
		To:          *toAddr,
		Inputs:      cv,
	}
	err = sm.evaluateDeployment(ctx, domain, tx)
	require.Error(t, err)
	domain.AssertExpectations(t)
}

// TestEvaluateDeployment_Success covers the full happy path: PrepareDeploy, local signer, key resolve,
// InvokeTransaction encode, ValidateTransaction, and PersistDeployDispatchBatch all succeed.
func TestEvaluateDeployment_Success(t *testing.T) {
	ctx := context.Background()
	mocks := newDeploymentTestMocks(t)
	sm := newSequencerManagerForDeploymentTesting(t, mocks)

	fnABI := &abi.Entry{Type: abi.Function, Name: "deploy", Inputs: abi.ParameterArray{}}
	cv, err := fnABI.Inputs.ParseJSONCtx(ctx, []byte("[]"))
	require.NoError(t, err)

	domain := componentsmocks.NewDomain(t)
	domain.EXPECT().PrepareDeploy(ctx, mock.Anything).Return(nil).Once()

	setupEvaluateDeploymentMocks(t, mocks, ctx)
	addr := pldtypes.RandAddress()
	mocks.keyManager.EXPECT().
		ResolveEthAddressBatchNewDatabaseTX(ctx, mock.Anything).
		Return([]*pldtypes.EthAddress{addr}, nil).Once()

	mocks.publicTxManager.EXPECT().
		ValidateTransaction(ctx, nil, mock.Anything).
		Return(nil).Once()

	mocks.syncPoints.EXPECT().
		PersistDeployDispatchBatch(ctx, mock.MatchedBy(func(b *syncpoints.DispatchBatch) bool {
			return b != nil && len(b.PublicDispatches) == 1 && len(b.PublicDispatches[0].PublicTxs) == 1
		})).
		Return(nil).Once()

	toAddr := pldtypes.RandAddress()
	tx := deployTxLocalSigner(uuid.New())
	tx.InvokeTransaction = &components.EthTransaction{
		FunctionABI: fnABI,
		To:          *toAddr,
		Inputs:      cv,
	}
	err = sm.evaluateDeployment(ctx, domain, tx)
	require.NoError(t, err)
	domain.AssertExpectations(t)
	mocks.syncPoints.AssertExpectations(t)
}
