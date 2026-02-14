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

package privatetxnmgr

import (
	"fmt"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

func newAssembleCoordinatorForTest(t *testing.T, timeout time.Duration) (*assembleCoordinator, *transactionFlowDependencyMocks) {
	log.InitConfig(&pldconf.LogDefaults)
	log.SetLevel("debug")
	mocks := newTransactionFlowDependencyMocks(t)
	ac := NewAssembleCoordinator(
		t.Context(),
		"node1",
		1,
		mocks.allComponents,
		mocks.domainSmartContract,
		mocks.domainContext,
		mocks.transportWriter,
		mocks.contractAddress,
		mocks.environment,
		timeout,
		mocks.localAssembler,
	).(*assembleCoordinator)
	t.Cleanup(ac.Stop)
	return ac, mocks
}

func waitAllAssembleRequestsComplete(t *testing.T, ac *assembleCoordinator) {
	for {
		ac.inflightMux.Lock()
		inflightCount := len(ac.inflightRequests)
		ac.inflightMux.Unlock()
		if t.Failed() || inflightCount == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestAssembleCoordinatorCompleteUnknownIDIsNonBlocking(t *testing.T) {
	ac, _ := newAssembleCoordinatorForTest(t, 5*time.Minute)
	ac.Complete("missing-request")
}

func TestAssembleCompletesLocalSuccessfully(t *testing.T) {
	ac, mocks := newAssembleCoordinatorForTest(t, 5*time.Minute)
	ac.Start()

	txID := uuid.New()

	assembly := &components.TransactionPreAssembly{}
	mocks.localAssembler.On("AssembleLocal", mock.Anything, mock.Anything, txID, assembly).
		Run(func(args mock.Arguments) {
			requestID := args[1].(string)
			ac.Complete(requestID)
		}).
		Return()

	ac.QueueAssemble(t.Context(), "node1" /* local */, txID, assembly)

	// Wait for the request to complete
	waitAllAssembleRequestsComplete(t, ac)
}

func TestAssembleCompletesRemoteSuccessfully(t *testing.T) {
	ac, mocks := newAssembleCoordinatorForTest(t, 5*time.Minute)
	ac.Start()

	txID := uuid.New()

	assembly := &components.TransactionPreAssembly{}
	mocks.domainContext.On("ExportSnapshot").Return([]byte{}, nil)
	mocks.environment.On("GetBlockHeight").Return(int64(12345))
	mocks.transportWriter.On("SendAssembleRequest", mock.Anything, "node2", mock.Anything, txID, mocks.contractAddress.String(), mock.Anything, mock.Anything, int64(12345)).
		Run(func(args mock.Arguments) {
			requestID := args[2].(string)
			ac.Complete(requestID)
		}).
		Return(nil)

	ac.QueueAssemble(t.Context(), "node2" /* remote */, txID, assembly)

	// Wait for the request to complete
	waitAllAssembleRequestsComplete(t, ac)
}

func TestAssembleRemoteFailExport(t *testing.T) {
	ac, mocks := newAssembleCoordinatorForTest(t, 5*time.Minute)
	ac.Start()

	txID := uuid.New()

	assembly := &components.TransactionPreAssembly{}
	mocks.domainContext.On("ExportSnapshot").Return([]byte{}, fmt.Errorf("pop"))

	ac.QueueAssemble(t.Context(), "node2" /* remote */, txID, assembly)

	// Wait for the request to complete
	waitAllAssembleRequestsComplete(t, ac)
}

func TestAssembleRemoteFailTransport(t *testing.T) {
	ac, mocks := newAssembleCoordinatorForTest(t, 5*time.Minute)
	ac.Start()

	txID := uuid.New()

	assembly := &components.TransactionPreAssembly{}
	mocks.domainContext.On("ExportSnapshot").Return([]byte{}, nil)
	mocks.environment.On("GetBlockHeight").Return(int64(12345))
	mocks.transportWriter.On("SendAssembleRequest", mock.Anything, "node2", mock.Anything, txID, mocks.contractAddress.String(), mock.Anything, mock.Anything, int64(12345)).
		Return(fmt.Errorf("pop"))

	ac.QueueAssemble(t.Context(), "node2" /* remote */, txID, assembly)

	// Wait for the request to complete
	waitAllAssembleRequestsComplete(t, ac)
}

func TestAssembleTimeout(t *testing.T) {
	ac, mocks := newAssembleCoordinatorForTest(t, 1*time.Millisecond)
	ac.Start()

	txID := uuid.New()

	assembly := &components.TransactionPreAssembly{}
	mocks.localAssembler.On("AssembleLocal", mock.Anything, mock.Anything, txID, assembly).
		Return()

	ac.QueueAssemble(t.Context(), "node1" /* local */, txID, assembly)

	// Wait for the request to timeout
	waitAllAssembleRequestsComplete(t, ac)
}

func TestAssembleTimeoutStopInFlight(t *testing.T) {
	ac, mocks := newAssembleCoordinatorForTest(t, 5*time.Minute)
	ac.Start()

	txID := uuid.New()

	assembly := &components.TransactionPreAssembly{}
	mocks.localAssembler.On("AssembleLocal", mock.Anything, mock.Anything, txID, assembly).Return()

	ac.QueueAssemble(t.Context(), "node1" /* local */, txID, assembly)

	ac.Stop()

	// Wait for the request to timeout
	waitAllAssembleRequestsComplete(t, ac)
}
