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

func (seq *sequencer) shutdown(ctx context.Context) {
	if seq.cancelCtx == nil {
		return
	}
	seq.cancelCtx()
	seq.coordinator.WaitForDone(ctx)
	seq.originator.WaitForDone(ctx)
	if seq.domainContext != nil {
		seq.domainContext.Close()
		seq.domainContext = nil
	}
}

// An instance of a sequencer (one instance per domain contract)
type sequencer struct {
	// The 3 main components of the sequencer
	originator      originator.Originator
	transportWriter transport.TransportWriter
	coordinator     coordinator.Coordinator
	cancelCtx       context.CancelFunc

	// Registered with StateManager; must Close after coordinator/originator stop (see statemgr.NewDomainContext).
	domainContext components.DomainContext

	// Sequencer attributes
	contractAddress string
	lastTXTime      time.Time
}

// heartbeatLoop runs for the lifetime of the sequencer, periodically queuing
// HeartbeatIntervalEvent to both the coordinator and originator state machines.
func (seq *sequencer) heartbeatLoop(ctx context.Context, heartbeatInterval time.Duration) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	hbEvent := &common.HeartbeatIntervalEvent{}
	seq.coordinator.QueueEvent(ctx, hbEvent)
	seq.originator.QueueEvent(ctx, hbEvent)

	for {
		select {
		case <-ticker.C:
			hbEvent := &common.HeartbeatIntervalEvent{}
			seq.coordinator.QueueEvent(ctx, hbEvent)
			seq.originator.QueueEvent(ctx, hbEvent)
		case <-ctx.Done():
			log.L(ctx).Debugf("heartbeat loop stopped for %s", seq.contractAddress)
			return
		}
	}
}

// Return the sequencer for the requested contract address, instantiating it first if this is its first use.
func (sMgr *sequencerManager) LoadSequencer(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction) (Sequencer, error) {
	return sMgr.loadSequencer(ctx, dbTX, contractAddr, domainAPI, tx, true)
}

// Return the sequencer only if it is already in memory. This never instantiates a new sequencer.
func (sMgr *sequencerManager) GetSequencer(ctx context.Context, contractAddr pldtypes.EthAddress) (Sequencer, error) {
	return sMgr.loadSequencer(ctx, nil, contractAddr, nil, nil, false)
}

func (sMgr *sequencerManager) loadSequencer(ctx context.Context, dbTX persistence.DBTX, contractAddr pldtypes.EthAddress, domainAPI components.DomainSmartContract, tx *components.PrivateTransaction, instantiate bool) (Sequencer, error) {
	var err error
	if instantiate && domainAPI == nil {
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
		if !instantiate {
			return nil, nil
		}

		//swap the read lock for a write lock
		sMgr.sequencersLock.RUnlock()
		readlock = false
		sMgr.sequencersLock.Lock()
		defer sMgr.sequencersLock.Unlock()

		//double check in case another goroutine has created the sequencer while we were waiting for the write lock
		if sMgr.sequencers[contractAddr.String()] == nil {

			log.L(ctx).Debugf("creating sequencer for contract address %s", contractAddr.String())

			// Are we handing this off to the sequencer now?
			// Locally we store mappings of contract address to originator/coordinator pair

			// Do we have space for another sequencer?
			if sMgr.targetActiveSequencersLimit > 0 && len(sMgr.sequencers) >= sMgr.targetActiveSequencersLimit {
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

			seqCtx, cancelCtx := context.WithCancel(log.WithComponent(sMgr.ctx, "sequencer"))
			seqCtx = log.WithLogField(seqCtx, "contract", contractAddr.String())

			// Create a transport writer for the sequencer to communicate with sequencers on other peers
			transportWriter := transport.NewTransportWriter(seqCtx, &contractAddr, sMgr.nodeName, sMgr.components.TransportManager(), sMgr.HandlePaladinMsg)

			engineIntegration := common.NewEngineIntegration(seqCtx, sMgr.components, sMgr.nodeName, domainAPI, dCtx, sMgr)
			sequencer := &sequencer{
				contractAddress: contractAddr.String(),
				transportWriter: transportWriter,
				cancelCtx:       cancelCtx,
				domainContext:   dCtx,
			}

			seqOriginator, err := originator.NewOriginator(seqCtx, sMgr.nodeName, transportWriter, engineIntegration, &contractAddr, sMgr.config, sMgr.metrics)
			if err != nil {
				cancelCtx()
				log.L(ctx).Errorf("failed to create sequencer originator for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			coordinator, err := coordinator.NewCoordinator(seqCtx,
				&contractAddr,
				domainAPI,
				dCtx,
				sMgr.components,
				nil,
				nil,
				transportWriter,
				common.RealClock(),
				engineIntegration,
				sMgr.syncPoints,
				sMgr.config,
				sMgr.nodeName,
				sMgr.metrics,
			)
			if err != nil {
				cancelCtx()
				log.L(ctx).Errorf("failed to create sequencer coordinator for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			sequencer.originator = seqOriginator
			sequencer.coordinator = coordinator
			sMgr.sequencers[contractAddr.String()] = sequencer

			go sequencer.heartbeatLoop(seqCtx, sMgr.heartbeatInterval)

			if tx != nil {
				sMgr.sequencers[contractAddr.String()].lastTXTime = time.Now()
			}

			log.L(ctx).Debugf("sqncr      | %s | started", contractAddr.String()[0:8])
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
		sequencer.shutdown(ctx)
	}
}

// Must be called within the sequencer's write lock
func (sMgr *sequencerManager) stopLowestPrioritySequencer(ctx context.Context) {
	log.L(ctx).Debugf("max concurrent sequencers reached, finding lowest priority sequencer to stop")
	if len(sMgr.sequencers) != 0 {
		// If any sequencers are already closing we can wait for them to close instead of stopping a different one
		for _, sequencer := range sMgr.sequencers {
			coordinatorState := sequencer.coordinator.GetCurrentState()
			switch coordinatorState {
			case coordinator.State_Flush, coordinator.State_Closing:
				// To avoid blocking the start of new sequencer that has caused us to purge the lowest priority one,
				// we don't wait for the closing ones to complete. The aim is to allow the node to remain stable while
				// still being responsive to new contract activity so a closing sequencer is allowed to page out in its
				// own time.
				log.L(ctx).Debugf("coordinator %s is closing, waiting for it to close", sequencer.contractAddress)
				return
			case coordinator.State_Idle, coordinator.State_Observing:
				originatorState := sequencer.originator.GetCurrentState()
				if originatorState == originator.State_Idle ||
					originatorState == originator.State_Observing {
					// This sequencer is already idle/observing on both coordinator and originator,
					// so we can page it out immediately. Caller holds the sequencers write lock.
					log.L(ctx).Debugf("stopping coordinator %s", sequencer.contractAddress)
					sequencer.shutdown(ctx)
					delete(sMgr.sequencers, sequencer.contractAddress)
					return
				}
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

		// Stop the lowest priority sequencer by emitting an event and waiting for it to move to closed
		log.L(ctx).Debugf("stopping coordinator %s", sequencers[0].contractAddress)
		sequencers[0].shutdown(ctx)
		delete(sMgr.sequencers, sequencers[0].contractAddress)
	}
}

func (sMgr *sequencerManager) cleanupIdleSequencers(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sMgr.removeIdleSequencers(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (sMgr *sequencerManager) removeIdleSequencers(ctx context.Context) {
	sMgr.sequencersLock.Lock()
	var toShutdown []*sequencer
	for addr, seq := range sMgr.sequencers {
		coordState := seq.coordinator.GetCurrentState()
		origState := seq.originator.GetCurrentState()
		if coordState == coordinator.State_Idle && origState == originator.State_Idle {
			log.L(ctx).Debugf("cleanup: stopping idle sequencer %s", addr)
			toShutdown = append(toShutdown, seq)
			delete(sMgr.sequencers, addr)
		}
	}
	sMgr.metrics.SetActiveSequencers(len(sMgr.sequencers))
	sMgr.sequencersLock.Unlock()

	for _, seq := range toShutdown {
		seq.shutdown(ctx)
	}
}
