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
	"fmt"
	"sort"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	coordTransaction "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

// Components needing to interact with the sequencer can make certain calls into
// the coordinator, the originator, or the transport writer
type Sequencer interface {
	GetCoordinator() coordinator.Coordinator
	GetOriginator() originator.SeqOriginator
	GetTransportWriter() transport.TransportWriter
}

func (seq *sequencer) GetCoordinator() coordinator.Coordinator {
	return seq.coordinator
}

func (seq *sequencer) GetOriginator() originator.SeqOriginator {
	return seq.originator
}

func (seq *sequencer) GetTransportWriter() transport.TransportWriter {
	return seq.transportWriter
}

// An instance of a sequencer (one instance per domain contract)
type sequencer struct {
	// The 3 main components of the sequencer
	originator      originator.SeqOriginator
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

			originator, err := originator.NewOriginator(sMgr.ctx, sMgr.nodeName, transportWriter, common.RealClock(), sMgr.engineIntegration, &contractAddr, sMgr.config, 15000, 10, sMgr.metrics)
			if err != nil {
				log.L(ctx).Errorf("failed to create sequencer originator for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			ctx, cancelCtx := context.WithCancel(sMgr.ctx)

			coordinator, err := coordinator.NewCoordinator(ctx,
				cancelCtx,
				&contractAddr,
				domainAPI,
				sMgr.components.TxManager(),
				transportWriter,
				common.RealClock(),
				sMgr.engineIntegration,
				sMgr.syncPoints,
				sMgr.config,
				sMgr.nodeName,
				sMgr.metrics,
				func(ctx context.Context, t *coordTransaction.Transaction) {
					// A transaction is ready to dispatch. Prepare & dispatch it.
					sMgr.dispatch(ctx, t, dCtx, transportWriter)
				},
				func(contractAddress *pldtypes.EthAddress, coordinatorNode string) {
					// A new coordinator became active or was confirmed as active. It might be us or it might be another node.
					// Update metrics and check if we need to stop one to stay within the configured max active coordinators
					sMgr.updateActiveCoordinators(sMgr.ctx)

					// TODO AM: this should be a queued event not a direct call
					// The originator needs to know to delegate transactions to the active coordinator
					err := originator.SetActiveCoordinator(sMgr.ctx, coordinatorNode)
					if err != nil {
						log.L(ctx).Errorf("failed to set active coordinator for contract %s: %s", contractAddr.String(), err)
						return
					}
				},
				func(contractAddress *pldtypes.EthAddress) {
					// A new coordinator became idle, perform any lifecycle tidy up
					sMgr.updateActiveCoordinators(sMgr.ctx)
				},
			)
			if err != nil {
				log.L(ctx).Errorf("failed to create sequencer coordinator for contract %s: %s", contractAddr.String(), err)
				return nil, err
			}

			sequencer.originator = originator
			sequencer.coordinator = coordinator
			sMgr.sequencers[contractAddr.String()] = sequencer

			// Start by populating the pool of originators with the endorsers of this transaction. At this point
			// we don't have anything else to use to determine who our candidate coordinators are.
			err = sMgr.setInitialCoordinator(ctx, tx, sequencer)
			if err != nil {
				return nil, err
			}

			if tx != nil {
				sMgr.sequencers[contractAddr.String()].lastTXTime = time.Now()
			}

			log.L(log.WithLogField(ctx, common.SEQUENCER_LOG_CATEGORY_FIELD, common.CATEGORY_LIFECYCLE)).Debugf("sqncr      | %s | started", contractAddr.String()[0:8])
		}
	} else {
		//TODO AM: is there anyway that we don't have an initial coordinator selected if we queue the event when it's first loaded
		// We already have a sequencer initialized but we might not have an initial coordinator selected
		// Start by populating the pool of originators with the endorsers of this transaction. At this point
		// we don't have anything else to use to determine who our candidate coordinators are.
		if sMgr.sequencers[contractAddr.String()].GetOriginator().GetCurrentCoordinator() == "" {
			// TODO AM: if this can be idempotent then we don't need to make the call to GetCurrentCoordinator
			err := sMgr.setInitialCoordinator(ctx, tx, sMgr.sequencers[contractAddr.String()])
			if err != nil {
				return nil, err
			}
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

func (sMgr *sequencerManager) setInitialCoordinator(ctx context.Context, tx *components.PrivateTransaction, sequencer *sequencer) error {

	if tx != nil && tx.PreAssembly != nil && tx.PreAssembly.RequiredVerifiers != nil {
		log.L(ctx).Debugf("setting initial coordinator for %s, updating origininator node pool to include required verifiers of transaction %s", sequencer.contractAddress, tx.ID.String())
		for _, verifier := range tx.PreAssembly.RequiredVerifiers {
			_, node, err := pldtypes.PrivateIdentityLocator(verifier.Lookup).Validate(ctx, sMgr.nodeName, false)
			if err != nil {
				return err
			}
			// TODO AM: I think this update and then set active needs to be an event to the coordinator
			// this causes the coordinator to call back into the sequencerManager- is this the point at which an event
			// should be queued for the originator?
			// I think it is- the next step is that the code today might assume the active coordinator is always set
			// do we need an initial state/retries if we don't- as we know it will come fairly imminently, just asynchronously
			sequencer.GetCoordinator().UpdateOriginatorNodePool(ctx, node)
		}

		// Get the best candidate for an initial coordinator, and use as the delegate for any originated transactions
		err := sequencer.GetOriginator().SetActiveCoordinator(sMgr.ctx, sequencer.GetCoordinator().GetActiveCoordinatorNode(sMgr.ctx, true))
		if err != nil {
			return err
		}
	}
	return nil
}

// Once we get here the sequencer has decided we have not gone too far ahead of the currently confirmed transactions, and are OK to
// dispatch a new transaction. There might be several transactions still in flight to the base ledger and maxDispatchAhead can be used
// to control how "optimistic" we are about submitting newly assembled and endorsed transactions.
func (sMgr *sequencerManager) dispatch(ctx context.Context, t *coordTransaction.Transaction, dCtx components.DomainContext, transportWriter transport.TransportWriter) {
	domainAPI, err := sMgr.components.DomainManager().GetSmartContractByAddress(ctx, sMgr.components.Persistence().NOTX(), t.GetContractAddress())
	if err != nil {
		log.L(ctx).Errorf("error getting domain API for contract %s: %s", t.GetContractAddress().String(), err)
		return
	}

	submitterSelection := domainAPI.ContractConfig().GetSubmitterSelection()

	if submitterSelection == prototk.ContractConfig_SUBMITTER_COORDINATOR {
		for _, endorsement := range t.GetPostAssembly().Endorsements {
			for _, constraint := range endorsement.Constraints {
				if constraint == prototk.AttestationResult_ENDORSER_MUST_SUBMIT {
					t.SetSigner(endorsement.Verifier.Lookup)
					break
				}
			}
		}
	}
	if t.GetSigner() == "" {
		if domainAPI.Domain().FixedSigningIdentity() != "" {
			t.SetSigner(domainAPI.Domain().FixedSigningIdentity())
		} else {
			t.SetSigner(fmt.Sprintf("domains.%s.submit.%s", t.GetContractAddress().String(), uuid.New()))
		}
	}
	log.L(ctx).Debugf("Transaction %s signer %s", t.GetID().String(), t.GetSigner())

	// Prepare the public or private transaction
	readTX := sMgr.components.Persistence().NOTX() // no DB transaction required here
	err = domainAPI.PrepareTransaction(dCtx, readTX, t.GetPrivateTransaction())
	if err != nil {
		log.L(ctx).Errorf("Error preparing transaction %s: %s", t.GetID().String(), err)
		return
	}

	dispatchBatch := &syncpoints.DispatchBatch{
		PublicDispatches: make([]*syncpoints.PublicDispatch, 0),
	}

	preparedTxnDistributions := make([]*components.PreparedTransactionWithRefs, 0)
	preparedTransaction := t.GetPrivateTransaction()
	publicTransactionsToSend := make([]*components.PrivateTransaction, 0)
	sequence := &syncpoints.PublicDispatch{}
	stateDistributions := make([]*components.StateDistribution, 0)
	localStateDistributions := make([]*components.StateDistributionWithData, 0)

	hasPublicTransaction := t.HasPreparedPublicTransaction()
	hasPrivateTransaction := t.HasPreparedPrivateTransaction()
	switch {
	case preparedTransaction.Intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPublicTransaction && !hasPrivateTransaction:
		log.L(ctx).Debugf("Result of transaction %s is a public transaction (gas=%d)", t.GetID(), *preparedTransaction.PreparedPublicTransaction.Gas)
		publicTransactionsToSend = append(publicTransactionsToSend, preparedTransaction)
		sequence.PrivateTransactionDispatches = append(sequence.PrivateTransactionDispatches, &syncpoints.DispatchPersisted{
			PrivateTransactionID: t.GetID().String(),
		})
	case preparedTransaction.Intent == prototk.TransactionSpecification_SEND_TRANSACTION && hasPrivateTransaction && !hasPublicTransaction:
		log.L(ctx).Debugf("Result of transaction %s is a chained private transaction", preparedTransaction.ID)
		addr := t.GetContractAddress()
		validatedPrivateTx, err := sMgr.components.TxManager().PrepareChainedPrivateTransaction(ctx, sMgr.components.Persistence().NOTX(), t.GetOriginalSender(), t.GetID(), t.GetDomain(), &addr, preparedTransaction.PreparedPrivateTransaction, pldapi.SubmitModeAuto)
		if err != nil {
			log.L(ctx).Errorf("error preparing transaction %s: %s", preparedTransaction.ID, err)
			// TODO: this is just an error situation for one transaction - this function is a batch function
			return
		}
		dispatchBatch.PrivateDispatches = append(dispatchBatch.PrivateDispatches, validatedPrivateTx)
	case preparedTransaction.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION && (hasPublicTransaction || hasPrivateTransaction):
		log.L(ctx).Debugf("Result of transaction %s is a prepared transaction public=%t private=%t", preparedTransaction.ID, hasPublicTransaction, hasPrivateTransaction)
		preparedTransactionWithRefs := mapPreparedTransaction(preparedTransaction)
		dispatchBatch.PreparedTransactions = append(dispatchBatch.PreparedTransactions, preparedTransactionWithRefs)

		// The prepared transaction needs to end up on the node that is able to submit it.
		preparedTxnDistributions = append(preparedTxnDistributions, preparedTransactionWithRefs)
	default:
		err = i18n.NewError(ctx, msgs.MsgSequencerInvalidPrepareOutcome, preparedTransaction.ID, preparedTransaction.Intent, hasPublicTransaction, hasPrivateTransaction)
		log.L(ctx).Errorf("error preparing transaction %s: %s", preparedTransaction.ID, err)
		return
	}

	stateDistributionBuilder := common.NewStateDistributionBuilder(sMgr.components, t.GetPrivateTransaction())
	sds, err := stateDistributionBuilder.Build(ctx)
	if err != nil {
		log.L(ctx).Errorf("error getting state distributions: %s", err)
	}

	for _, sd := range sds.Remote {
		log.L(ctx).Debugf("Adding remote state distribution %+v", sd.StateDistribution)
		stateDistributions = append(stateDistributions, &sd.StateDistribution)
	}
	localStateDistributions = append(localStateDistributions, sds.Local...)

	// Now we have the payloads, we can prepare the submission
	publicTransactionEngine := sMgr.components.PublicTxManager()

	// we may or may not have any transactions to send depending on the submit mode
	if len(publicTransactionsToSend) == 0 {
		log.L(ctx).Debugf("No public transactions to send for TX %s", t.GetID().String())
	} else {
		contractAddr := t.GetContractAddress()
		signers := make([]string, len(publicTransactionsToSend))
		for i, pt := range publicTransactionsToSend {
			unqualifiedSigner, err := pldtypes.PrivateIdentityLocator(pt.Signer).Identity(ctx)
			if err != nil {
				err = i18n.WrapError(ctx, err, msgs.MsgSequencerInternalError, err)
				log.L(ctx).Error(err)
				return
			}

			signers[i] = unqualifiedSigner
		}
		keyMgr := sMgr.components.KeyManager()
		resolvedAddrs, err := keyMgr.ResolveEthAddressBatchNewDatabaseTX(ctx, signers)
		if err != nil {
			log.L(ctx).Errorf("failed to resolve signers for public transactions: %s", err)
			return
		}

		publicTXs := make([]*components.PublicTxSubmission, len(publicTransactionsToSend))
		for i, pt := range publicTransactionsToSend {
			log.L(ctx).Debugf("DispatchTransactions: creating PublicTxSubmission from %s", pt.Signer)
			publicTXs[i] = &components.PublicTxSubmission{
				Bindings: []*components.PaladinTXReference{{TransactionID: pt.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
				PublicTxInput: pldapi.PublicTxInput{
					From:            resolvedAddrs[i],
					To:              &contractAddr,
					PublicTxOptions: pt.PreparedPublicTransaction.PublicTxOptions,
				},
			}

			// MRW TODO - We currently issue state machine CollectEvents from the publix TX manager, but we could arguably do it here.

			data, err := pt.PreparedPublicTransaction.ABI[0].EncodeCallDataJSONCtx(ctx, pt.PreparedPublicTransaction.Data)
			if err != nil {
				log.L(ctx).Errorf("failed to encode call data for public transaction %s: %s", pt.ID, err)
				return
			}
			publicTXs[i].Data = pldtypes.HexBytes(data)

			log.L(ctx).Tracef("Validating public transaction %s", pt.ID.String())
			err = publicTransactionEngine.ValidateTransaction(ctx, sMgr.components.Persistence().NOTX(), publicTXs[i])
			if err != nil {
				log.L(ctx).Errorf("failed to encode call data for public transaction %s: %s", pt.ID, err)
				return
			}
		}
		sequence.PublicTxs = publicTXs
		dispatchBatch.PublicDispatches = append(dispatchBatch.PublicDispatches, sequence)
	}

	// Determine if there are any local nullifiers that need to be built and put into the domain context
	// before we persist the dispatch batch
	localNullifiers, err := sMgr.BuildNullifiers(ctx, localStateDistributions)
	if err == nil && len(localNullifiers) > 0 {
		err = dCtx.UpsertNullifiers(localNullifiers...)
	}
	if err != nil {
		log.L(ctx).Errorf("error building nullifiers: %s", err)
		return
	}

	log.L(ctx).Debugf("Persisting & deploying batch. %d public transactions, %d private transactions, %d prepared transactions", len(dispatchBatch.PublicDispatches), len(dispatchBatch.PrivateDispatches), len(dispatchBatch.PreparedTransactions))
	err = sMgr.syncPoints.PersistDispatchBatch(dCtx, t.GetContractAddress(), dispatchBatch, stateDistributions, preparedTxnDistributions)
	if err != nil {
		log.L(ctx).Errorf("error persisting batch: %s", err)
		return
	}

	err = transportWriter.SendDispatched(ctx, t.Originator(), uuid.New(), t.GetTransactionSpecification())
	if err != nil {
		log.L(ctx).Errorf("failed to send dispatched event for transaction %s: %s", t.GetID(), err)
		return
	}

	// We also need to trigger ourselves for any private TX we chained
	for _, dispatch := range dispatchBatch.PrivateDispatches {
		// Create a new DB transaction and handle the new transaction
		err = sMgr.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTx persistence.DBTX) error {
			return sMgr.HandleNewTx(ctx, dbTx, dispatch.NewTransaction)
		})
		if err != nil {
			log.L(ctx).Errorf("error handling new transaction: %v", err)
			return
		}
	}
	log.L(ctx).Debugf("Chained %d private transactions", len(dispatchBatch.PrivateDispatches))
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
