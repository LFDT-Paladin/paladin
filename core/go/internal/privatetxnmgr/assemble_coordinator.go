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
	cancelCtx            context.CancelFunc
	nodeName             string
	newRequests          chan *assembleRequest
	inflightMux          sync.Mutex
	inflightRequests     map[string]*assembleRequest
	components           components.AllComponents
	domainAPI            components.DomainSmartContract
	domainContext        components.DomainContext
	transportWriter      ptmgrtypes.TransportWriter
	contractAddress      pldtypes.EthAddress
	sequencerEnvironment ptmgrtypes.SequencerEnvironment
	requestTimeout       time.Duration
	localAssembler       ptmgrtypes.LocalAssembler
}

type assembleRequest struct {
	requestID              string
	ac                     *assembleCoordinator
	assemblingNode         string
	assembleCoordinator    *assembleCoordinator
	transactionID          uuid.UUID
	transactionPreassembly *components.TransactionPreAssembly
	done                   chan struct{}
}

func NewAssembleCoordinator(ctx context.Context, nodeName string, maxPendingRequests int, components components.AllComponents, domainAPI components.DomainSmartContract, domainContext components.DomainContext, transportWriter ptmgrtypes.TransportWriter, contractAddress pldtypes.EthAddress, sequencerEnvironment ptmgrtypes.SequencerEnvironment, requestTimeout time.Duration, localAssembler ptmgrtypes.LocalAssembler) ptmgrtypes.AssembleCoordinator {
	ac := &assembleCoordinator{
		ctx:                  ctx,
		nodeName:             nodeName,
		newRequests:          make(chan *assembleRequest, maxPendingRequests),
		inflightRequests:     make(map[string]*assembleRequest),
		components:           components,
		domainAPI:            domainAPI,
		domainContext:        domainContext,
		transportWriter:      transportWriter,
		contractAddress:      contractAddress,
		sequencerEnvironment: sequencerEnvironment,
		requestTimeout:       requestTimeout,
		localAssembler:       localAssembler,
	}
	ac.ctx, ac.cancelCtx = context.WithCancel(ctx)
	return ac
}

func (ac *assembleCoordinator) Complete(requestID string) {

	log.L(ac.ctx).Debugf("AssembleCoordinator:Commit %s", requestID)

	ac.inflightMux.Lock()
	request := ac.inflightRequests[requestID]
	ac.inflightMux.Unlock()

	if request == nil {
		log.L(ac.ctx).Warnf("AssembleCoordinator:Commit request %s no longer in-flight", requestID)
		return
	}
	// There's no actual response here - we're just waiting for it to be done
	close(request.done)
}

func (ac *assembleCoordinator) Start() {
	log.L(ac.ctx).Info("Starting AssembleCoordinator")
	go func() {
		for {
			select {
			case req := <-ac.newRequests:
				if req.assemblingNode == "" || req.assemblingNode == ac.nodeName {
					req.processLocal(ac.ctx, req.requestID)
				} else {
					err := req.processRemote(ac.ctx, req.assemblingNode, req.requestID)
					if err != nil {
						log.L(ac.ctx).Errorf("AssembleCoordinator request failed: %s", err)
						//we failed sending the request so we continue to the next request
						// without waiting for this one to complete
						// the sequencer event loop is responsible for requesting a new assemble
						req.cleanup()
						continue
					}
				}

				//The actual response is processed on the sequencer event loop.  We just need to know when it is safe to proceed
				// to the next request
				req.waitForDone()
			case <-ac.ctx.Done():
				log.L(ac.ctx).Info("AssembleCoordinator loop exit due to canceled context")
				return
			}
		}
	}()
}

func (ar *assembleRequest) cleanup() {
	log.L(ar.ac.ctx).Debugf("AssembleCoordinator:cleanup %s", ar.requestID)
	ar.ac.inflightMux.Lock()
	delete(ar.ac.inflightRequests, ar.requestID)
	ar.ac.inflightMux.Unlock()
}

func (ar *assembleRequest) waitForDone() {
	ac := ar.ac
	log.L(ac.ctx).Debugf("AssembleCoordinator:waitForDone %s", ar.requestID)

	// Always remove from the inflight request mux - including on timeout.
	// Complete() will log if the completion comes after we give up waiting
	defer ar.cleanup()

	// wait for the response or a timeout
	timeoutTimer := time.NewTimer(ac.requestTimeout)
out:
	for {
		select {
		case <-ar.done:
			log.L(ac.ctx).Debugf("AssembleCoordinator:waitForDone received notification of completion %s", ar.requestID)
			return
		case <-ac.ctx.Done():
			log.L(ac.ctx).Info("AssembleCoordinator:waitForDone loop exit due to canceled context")
			return
		case <-timeoutTimer.C:
			log.L(ac.ctx).Errorf("AssembleCoordinator:waitForDone request timeout for request %s", ar.requestID)
			//sequencer event loop is responsible for requesting a new assemble
			break out
		}
	}

}

// Cancels waiting for in-flight requests
func (ac *assembleCoordinator) Stop() {
	ac.cancelCtx()
}

// TODO really need to figure out the separation between PrivateTxManager and DomainManager
// to allow us to do the assemble on a separate thread and without worrying about locking the PrivateTransaction objects
// we copy the pertinent structures out of the PrivateTransaction and pass them to the assemble thread
// and then use them to create another private transaction object that is passed to the domain manager which then just unpicks it again
func (ac *assembleCoordinator) QueueAssemble(ctx context.Context, assemblingNode string, transactionID uuid.UUID, transactionPreAssembly *components.TransactionPreAssembly) {

	req := &assembleRequest{
		requestID:              uuid.New().String(),
		ac:                     ac,
		assemblingNode:         assemblingNode,
		assembleCoordinator:    ac,
		transactionID:          transactionID,
		transactionPreassembly: transactionPreAssembly,
		done:                   make(chan struct{}),
	}

	ac.inflightMux.Lock()
	ac.inflightRequests[req.requestID] = req
	ac.inflightMux.Unlock()

	ac.newRequests <- req
	log.L(ctx).Debugf("QueueAssemble: assemble request %s for transaction %s queued", req.requestID, transactionID)

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
