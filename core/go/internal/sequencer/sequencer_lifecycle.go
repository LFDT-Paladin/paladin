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
	"sort"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// Components needing to interact with the sequencer can make certain calls into
// the coordinator, the originator, or the transport writer
type Sequencer interface {
	GetCoordinator() coordinator.Coordinator
	GetOriginator() originator.Originator
	GetTransportWriter() transport.TransportWriter
}

func (seq *sequencer) GetCoordinator() coordinator.Coordinator {
	return seq.coordinator
}

func (seq *sequencer) GetOriginator() originator.Originator {
	return seq.originator
}

func (seq *sequencer) GetTransportWriter() transport.TransportWriter {
	return seq.transportWriter
}

// An instance of a sequencer (one instance per domain contract)
type sequencer struct {
	// The 3 main components of the sequencer
	originator      originator.Originator
	transportWriter transport.TransportWriter
	coordinator     coordinator.Coordinator

	// Sequencer attributes
	contractAddress string
	lastTXTime      time.Time
}

// Return the sequencer for the requested contract address, instantiating it first if this is its first use
func (sMgr *sequencerManager) LoadSequencer(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction) (Sequencer, error) {
	var err error
	if domainAPI == nil {
		// Does a domain exist at this address?
		_, err = sMgr.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
		if err != nil {
			// Treat as a valid case, let the caller decide if it is or not
			log.L(ctx).Debugf("no sequencer found for contract %s, assuming contract deploy: %s", contractAddr, err)
			return nil, nil
		}
	}

	readlock := true
	sMgr.sequencersLock.RLock()
	defer func() {
		if readlock {
			sMgr.sequencersLock.RUnlock()
		}
	}()

	if sMgr.sequencers[contractAddr.String()] == nil {
		//swap the read lock for a write lock
		sMgr.sequencersLock.RUnlock()
		readlock = false
		sMgr.sequencersLock.Lock()
		defer sMgr.sequencersLock.Unlock()

		//double check in case another goroutine has created the sequencer while we were waiting for the write lock
		if sMgr.sequencers[contractAddr.String()] == nil {

			log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("creating sequencer for contract address %s", contractAddr.String())

			// Are we handing this off to the sequencer now?
			// Locally we store mappings of contract address to originator/coordinator pair

			// Do we have space for another sequencer?
			if sMgr.targetActiveSequencersLimit > 0 && len(sMgr.sequencers) > sMgr.targetActiveSequencersLimit {
				log.L(ctx).Debugf("max concurrent sequencers reached, stopping lowest priority sequencer")
				sMgr.stopLowestPrioritySequencer(ctx)
			}
			sMgr.metrics.SetActiveSequencers(len(sMgr.sequencers))

			if tx == nil {
				log.L(ctx).Debugf("No TX provided to create sequencer for contract %s", contractAddr.String())
			}

			domainAPI, err := sMgr.components.DomainManager().GetSmartContractByAddress(ctx, sMgr.components.Persistence().NOTX(), contractAddr)
			if err != nil {
				log.L(ctx).Errorf("failed to get domain API for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			if domainAPI == nil {
				err := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "No domain provided to create sequencer for contract %s", contractAddr.String())
				log.L(ctx).Error(err)
				return nil, err
			}

			// Create a new domain context for the sequencer. This will be re-used for the lifetime of the sequencer
			dCtx := sMgr.components.StateManager().NewDomainContext(sMgr.ctx, domainAPI.Domain(), contractAddr)

			// Create a 2nd domain context for assembling remote transactions
			delegateDomainContext := sMgr.components.StateManager().NewDomainContext(sMgr.ctx, domainAPI.Domain(), contractAddr)

			// Create a transport writer for the sequencer to communicate with sequencers on other peers
			transportWriter := transport.NewTransportWriter(&contractAddr, sMgr.nodeName, sMgr.components.TransportManager(), sMgr.HandlePaladinMsg)

			sMgr.engineIntegration = common.NewEngineIntegration(sMgr.ctx, sMgr.components, sMgr.nodeName, domainAPI, dCtx, delegateDomainContext, sMgr)
			sequencer := &sequencer{
				contractAddress: contractAddr.String(),
				transportWriter: transportWriter,
			}

			seqOriginator, err := originator.NewOriginator(sMgr.ctx, sMgr.nodeName, transportWriter, common.RealClock(), sMgr.engineIntegration, &contractAddr, sMgr.config, 15000, 10, sMgr.metrics)
			if err != nil {
				log.L(ctx).Errorf("failed to create sequencer originator for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			// Start by populating the pool of originators with the endorsers of this transaction. At this point
			// we don't have anything else to use to determine who our candidate coordinators are.
			initialOriginatorNodes, err := sMgr.getOriginatorNodesFromTx(ctx, tx)
			if err != nil {
				return nil, err
			}

			coordinator := coordinator.NewCoordinator(
				sMgr.ctx,
				&contractAddr,
				domainAPI,
				sMgr.components.TxManager(),
				sMgr.components.KeyManager(),
				sMgr.components.PublicTxManager(),
				transportWriter,
				common.RealClock(),
				sMgr.engineIntegration,
				sMgr.syncPoints,
				initialOriginatorNodes,
				sMgr.config,
				sMgr.nodeName,
				sMgr.metrics,
				dCtx,
				sMgr.BuildNullifiers,
				sMgr.HandleNewTransactions,
				func(contractAddress *pldtypes.EthAddress, coordinatorNode string) {
					// A new coordinator became active or was confirmed as active. It might be us or it might be another node.
					// Update metrics and check if we need to stop one to stay within the configured max active coordinators
					sMgr.updateActiveCoordinators(sMgr.ctx)

					// The originator needs to know to delegate transactions to the active coordinator
					seqOriginator.QueueEvent(sMgr.ctx, &originator.ActiveCoordinatorUpdatedEvent{
						BaseEvent:   common.BaseEvent{EventTime: time.Now()},
						Coordinator: coordinatorNode,
					})
				},
				func(contractAddress *pldtypes.EthAddress) {
					// A new coordinator became idle, perform any lifecycle tidy up
					sMgr.updateActiveCoordinators(sMgr.ctx)
				},
			)
			coordinator.Start()

			sequencer.originator = seqOriginator
			sequencer.coordinator = coordinator
			sMgr.sequencers[contractAddr.String()] = sequencer

			if tx != nil {
				sMgr.sequencers[contractAddr.String()].lastTXTime = time.Now()
			}

			log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("sqncr      | %s | started", contractAddr.String()[0:8])
		}
	} else {
		seq := sMgr.sequencers[contractAddr.String()]
		// When the sequencer already existed, it may have been created with tx=nil (e.g. on first
		// AssembleRequest) and have an empty originator pool. Queue an event so the coordinator
		// updates its pool with this transaction's endorsers and, if still in State_Initial, can
		// select an active coordinator.
		originatorNodes, err := sMgr.getOriginatorNodesFromTx(ctx, tx)
		if err != nil {
			return nil, err
		}
		if len(originatorNodes) > 0 {
			seq.GetCoordinator().QueueEvent(ctx, &coordinator.OriginatorNodePoolUpdateRequestedEvent{
				BaseEvent: common.BaseEvent{EventTime: time.Now()},
				Nodes:     originatorNodes,
			})
		}
	}

	if tx != nil {
		sMgr.sequencers[contractAddr.String()].lastTXTime = time.Now()
	}

	return sMgr.sequencers[contractAddr.String()], nil
}

func (sMgr *sequencerManager) StopAllSequencers(ctx context.Context) {
	sMgr.sequencersLock.Lock()
	defer sMgr.sequencersLock.Unlock()
	for _, sequencer := range sMgr.sequencers {
		sequencer.GetCoordinator().Stop()
		sequencer.GetOriginator().Stop()
	}
}

func (sMgr *sequencerManager) getOriginatorNodesFromTx(ctx context.Context, tx *components.PrivateTransaction) ([]string, error) {
	if tx == nil || tx.PreAssembly == nil || tx.PreAssembly.RequiredVerifiers == nil {
		return nil, nil
	}
	log.L(ctx).Debugf("setting initial coordinator, updating originator node pool to include required verifiers of transaction %s", tx.ID.String())
	nodes := make([]string, 0, len(tx.PreAssembly.RequiredVerifiers))
	for _, verifier := range tx.PreAssembly.RequiredVerifiers {
		_, node, err := pldtypes.PrivateIdentityLocator(verifier.Lookup).Validate(ctx, sMgr.nodeName, false)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// Must be called within the sequencer's write lock
func (sMgr *sequencerManager) stopLowestPrioritySequencer(ctx context.Context) {
	readlock := true
	sMgr.sequencersLock.RLock()
	defer func() {
		if readlock {
			sMgr.sequencersLock.RUnlock()
		}
	}()
	log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("max concurrent sequencers reached, finding lowest priority sequencer to stop")
	if len(sMgr.sequencers) != 0 {
		// If any sequencers are already closing we can wait for them to close instead of stopping a different one
		for _, sequencer := range sMgr.sequencers {
			if sequencer.coordinator.GetCurrentState() == coordinator.State_Flush ||
				sequencer.coordinator.GetCurrentState() == coordinator.State_Closing {

				// To avoid blocking the start of new sequencer that has caused us to purge the lowest priority one,
				// we don't wait for the closing ones to complete. The aim is to allow the node to remain stable while
				// still being responsive to new contract activity so a closing sequencer is allowed to page out in its
				// own time.
				log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("coordinator %s is closing, waiting for it to close", sequencer.contractAddress)
				return
			} else if sequencer.coordinator.GetCurrentState() == coordinator.State_Idle ||
				sequencer.coordinator.GetCurrentState() == coordinator.State_Observing {
				// This sequencer is already idle or observing so we can page it out immediately

				// swap the read lock for a write lock
				sMgr.sequencersLock.RUnlock()
				readlock = false
				sMgr.sequencersLock.Lock()
				defer sMgr.sequencersLock.Unlock()

				log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("stopping coordinator %s", sequencer.contractAddress)
				sequencer.coordinator.Stop()
				sequencer.originator.Stop()
				delete(sMgr.sequencers, sequencer.contractAddress)
				return
			}
		}

		// Order existing sequencers by LRU time
		sequencers := make([]*sequencer, 0)
		for _, sequencer := range sMgr.sequencers {
			sequencers = append(sequencers, sequencer)
		}
		sort.Slice(sequencers, func(i, j int) bool {
			return sequencers[i].lastTXTime.Before(sequencers[j].lastTXTime)
		})

		// swap the read lock for a write lock
		sMgr.sequencersLock.RUnlock()
		readlock = false
		sMgr.sequencersLock.Lock()
		defer sMgr.sequencersLock.Unlock()

		// Stop the lowest priority sequencer by emitting an event and waiting for it to move to closed
		log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("stopping coordinator %s", sequencers[0].contractAddress)
		sequencers[0].coordinator.Stop()
		sequencers[0].originator.Stop()
		delete(sMgr.sequencers, sequencers[0].contractAddress)
	}
}

func (sMgr *sequencerManager) updateActiveCoordinators(ctx context.Context) {
	log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("checking if max concurrent coordinators limit reached")

	readlock := true
	sMgr.sequencersLock.RLock()
	defer func() {
		if readlock {
			sMgr.sequencersLock.RUnlock()
		}
	}()

	activeCoordinators := 0
	// If any sequencers are already closing we can wait for them to close instead of stopping a different one
	for _, sequencer := range sMgr.sequencers {
		log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_STATE)).Debugf("coord    | %s   | %s", sequencer.contractAddress[0:8], sequencer.coordinator.GetCurrentState())
		if sequencer.coordinator.GetCurrentState() == coordinator.State_Active {
			activeCoordinators++
		}
	}

	sMgr.metrics.SetActiveCoordinators(activeCoordinators)

	if activeCoordinators >= sMgr.targetActiveCoordinatorsLimit {
		log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("%d coordinators currently active, max concurrent coordinators reached, asking the lowest priority coordinator to hand over to another node", activeCoordinators)
		// Order existing sequencers by LRU time
		sequencers := make([]*sequencer, 0)
		for _, sequencer := range sMgr.sequencers {
			sequencers = append(sequencers, sequencer)
		}
		sort.Slice(sequencers, func(i, j int) bool {
			return sequencers[i].lastTXTime.Before(sequencers[j].lastTXTime)
		})

		// swap the read lock for a write lock
		sMgr.sequencersLock.RUnlock()
		readlock = false
		sMgr.sequencersLock.Lock()
		defer sMgr.sequencersLock.Unlock()

		// Stop the lowest priority coordinator by emitting an event asking it to handover to another coordinator
		log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("stopping coordinator %s", sequencers[0].contractAddress)
		sequencers[0].coordinator.Stop()
		sequencers[0].originator.Stop()
		delete(sMgr.sequencers, sequencers[0].contractAddress)
	} else {
		log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("%d coordinators within max coordinator limit %d", activeCoordinators, sMgr.targetActiveCoordinatorsLimit)
	}
}
