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

package privatetxnmgr

import (
	"context"
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

type assembleCoordinator struct {
	ctx                  context.Context
	nodeName             string
	requests             chan *assembleRequest
	stopProcess          chan struct{}
	stopProcessOnce      sync.Once
	inflightLock         sync.Mutex
	inflight             map[string]chan struct{}
	components           components.AllComponents
	domainAPI            components.DomainSmartContract
	domainContext        components.DomainContext
	transportWriter      ptmgrtypes.TransportWriter
	contractAddress      pldtypes.EthAddress
	sequencerEnvironment ptmgrtypes.SequencerEnvironment
	requestTimeout       time.Duration
	localAssembler       ptmgrtypes.LocalAssembler
	timerFactory         func(time.Duration) coordinatorTimer
}

// Timer abstraction (allows mocking for test)
type coordinatorTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type defaultCoordinatorTimer struct {
	timer *time.Timer
}

func (t *defaultCoordinatorTimer) C() <-chan time.Time {
	return t.timer.C
}

func (t *defaultCoordinatorTimer) Stop() bool {
	return t.timer.Stop()
}

func newDefaultTimer(d time.Duration) coordinatorTimer {
	return &defaultCoordinatorTimer{timer: time.NewTimer(d)}
}

type assembleRequest struct {
	assemblingNode         string
	assembleCoordinator    *assembleCoordinator
	transactionID          uuid.UUID
	transactionPreassembly *components.TransactionPreAssembly
}

func NewAssembleCoordinator(ctx context.Context, nodeName string, maxPendingRequests int, components components.AllComponents, domainAPI components.DomainSmartContract, domainContext components.DomainContext, transportWriter ptmgrtypes.TransportWriter, contractAddress pldtypes.EthAddress, sequencerEnvironment ptmgrtypes.SequencerEnvironment, requestTimeout time.Duration, localAssembler ptmgrtypes.LocalAssembler) ptmgrtypes.AssembleCoordinator {
	return &assembleCoordinator{
		ctx:                  ctx,
		nodeName:             nodeName,
		stopProcess:          make(chan struct{}),
		requests:             make(chan *assembleRequest, maxPendingRequests),
		inflight:             make(map[string]chan struct{}),
		components:           components,
		domainAPI:            domainAPI,
		domainContext:        domainContext,
		transportWriter:      transportWriter,
		contractAddress:      contractAddress,
		sequencerEnvironment: sequencerEnvironment,
		requestTimeout:       requestTimeout,
		localAssembler:       localAssembler,
		timerFactory:         newDefaultTimer,
	}
}

func (ac *assembleCoordinator) Complete(requestID string) {
	if ac.completeInflight(requestID) {
		log.L(ac.ctx).Debugf("AssembleCoordinator:Complete %s", requestID)
		return
	}
	log.L(ac.ctx).Debugf("AssembleCoordinator:Complete ignored for unknown or stale request %s", requestID)
}

func (ac *assembleCoordinator) Start() {
	log.L(ac.ctx).Info("Starting AssembleCoordinator")
	go func() {
		for {
			select {
			case req := <-ac.requests:
				requestID := uuid.New().String()
				doneCh := ac.registerInflight(requestID)
				if req.assemblingNode == "" || req.assemblingNode == ac.nodeName {
					req.processLocal(ac.ctx, requestID)
				} else {
					err := req.processRemote(ac.ctx, req.assemblingNode, requestID)
					if err != nil {
						log.L(ac.ctx).Errorf("AssembleCoordinator request failed: %s", err)
						ac.removeInflight(requestID)
						//we failed sending the request so we continue to the next request
						// without waiting for this one to complete
						// the sequencer event loop is responsible for requesting a new assemble
						continue
					}
				}

				//The actual response is processed on the sequencer event loop.  We just need to know when it is safe to proceed
				// to the next request
				ac.waitForDone(requestID, doneCh)
			case <-ac.stopProcess:
				log.L(ac.ctx).Info("assembleCoordinator loop process stopped")
				return
			case <-ac.ctx.Done():
				log.L(ac.ctx).Info("AssembleCoordinator loop exit due to canceled context")
				return
			}
		}
	}()
}

func (ac *assembleCoordinator) waitForDone(requestID string, doneCh <-chan struct{}) {
	log.L(ac.ctx).Debugf("AssembleCoordinator:waitForDone %s", requestID)

	// wait for the response or a timeout
	timeoutTimer := ac.timerFactory(ac.requestTimeout)
	defer func() {
		ac.removeInflight(requestID)
		if !timeoutTimer.Stop() {
			select {
			case <-timeoutTimer.C():
			default:
			}
		}
	}()
out:
	for {
		select {
		case <-doneCh:
			log.L(ac.ctx).Debugf("AssembleCoordinator:waitForDone received notification of completion %s", requestID)
			break out
		case <-ac.stopProcess:
			log.L(ac.ctx).Info("AssembleCoordinator:waitForDone stopped")
			return
		case <-ac.ctx.Done():
			log.L(ac.ctx).Info("AssembleCoordinator:waitForDone loop exit due to canceled context")
			return
		case <-timeoutTimer.C():
			log.L(ac.ctx).Errorf("AssembleCoordinator:waitForDone request timeout for request %s", requestID)
			//sequencer event loop is responsible for requesting a new assemble
			break out
		}
	}
	log.L(ac.ctx).Debugf("AssembleCoordinator:waitForDone done %s", requestID)

}

func (ac *assembleCoordinator) Stop() {
	ac.stopProcessOnce.Do(func() {
		close(ac.stopProcess)
	})
}

func (ac *assembleCoordinator) registerInflight(requestID string) chan struct{} {
	ac.inflightLock.Lock()
	defer ac.inflightLock.Unlock()
	doneCh := make(chan struct{})
	ac.inflight[requestID] = doneCh
	return doneCh
}

func (ac *assembleCoordinator) completeInflight(requestID string) bool {
	ac.inflightLock.Lock()
	doneCh, found := ac.inflight[requestID]
	if found {
		delete(ac.inflight, requestID)
	}
	ac.inflightLock.Unlock()
	if found {
		close(doneCh)
	}
	return found
}

func (ac *assembleCoordinator) removeInflight(requestID string) {
	ac.inflightLock.Lock()
	delete(ac.inflight, requestID)
	ac.inflightLock.Unlock()
}

// TODO really need to figure out the separation between PrivateTxManager and DomainManager
// to allow us to do the assemble on a separate thread and without worrying about locking the PrivateTransaction objects
// we copy the pertinent structures out of the PrivateTransaction and pass them to the assemble thread
// and then use them to create another private transaction object that is passed to the domain manager which then just unpicks it again
func (ac *assembleCoordinator) QueueAssemble(ctx context.Context, assemblingNode string, transactionID uuid.UUID, transactionPreAssembly *components.TransactionPreAssembly) {

	ac.requests <- &assembleRequest{
		assemblingNode:         assemblingNode,
		assembleCoordinator:    ac,
		transactionID:          transactionID,
		transactionPreassembly: transactionPreAssembly,
	}
	log.L(ctx).Debugf("QueueAssemble: assemble request for %s queued", transactionID)

}

func (req *assembleRequest) processLocal(ctx context.Context, requestID string) {
	log.L(ctx).Debug("assembleRequest:processLocal")

	req.assembleCoordinator.localAssembler.AssembleLocal(ctx, requestID, req.transactionID, req.transactionPreassembly)

	log.L(ctx).Debug("assembleRequest:processLocal complete")

}

func (req *assembleRequest) processRemote(ctx context.Context, assemblingNode string, requestID string) error {

	//Assemble may require a call to another node ( in the case we have been delegated to coordinate transaction for other nodes)
	//Usually, they will get sent to us already assembled but there may be cases where we need to re-assemble
	// so this needs to be an async step
	// however, there must be only one assemble in progress at a time or else there is a risk that 2 transactions could chose to spend the same state
	//   (TODO - maybe in future, we could further optimize this and allow multiple assembles to be in progress if we can assert that they are not presented with the same available states)
	//   However, before we do that, we really need to sort out the separation of concerns between the domain manager, state store and private transaction manager and where the responsibility to single thread the assembly stream(s) lies

	log.L(ctx).Debugf("assembleRequest:processRemote requestID %s", requestID)

	stateLocksJSON, err := req.assembleCoordinator.domainContext.ExportSnapshot()
	if err != nil {
		return err
	}

	contractAddressString := req.assembleCoordinator.contractAddress.String()
	blockHeight := req.assembleCoordinator.sequencerEnvironment.GetBlockHeight()
	log.L(ctx).Debugf("assembleRequest:processRemote Assembling transaction %s on node %s", req.transactionID.String(), assemblingNode)

	//send a request to the node that is responsible for assembling this transaction
	err = req.assembleCoordinator.transportWriter.SendAssembleRequest(ctx, assemblingNode, requestID, req.transactionID, contractAddressString, req.transactionPreassembly, stateLocksJSON, blockHeight)
	if err != nil {
		log.L(ctx).Errorf("assembleRequest:processRemote error from sendAssembleRequest: %s", err)
		return err
	}
	return nil
}
