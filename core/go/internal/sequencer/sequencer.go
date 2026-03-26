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
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	coordinatorTx "github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LFDT-Paladin/paladin/core/internal/msgs"

	"github.com/LFDT-Paladin/paladin/core/pkg/blockindexer"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
)

type sequencerManager struct {
	ctx                           context.Context
	cancelCtx                     func()
	config                        *pldconf.SequencerConfig
	components                    components.AllComponents
	nodeName                      string
	sequencersLock                sync.RWMutex
	syncPoints                    syncpoints.SyncPoints
	metrics                       metrics.DistributedSequencerMetrics
	sequencers                    map[string]*sequencer
	blockHeight                   int64
	blockHeightMutex              sync.RWMutex
	engineIntegration             common.EngineIntegration
	targetActiveCoordinatorsLimit int // Max number of contracts this node aims to concurrently act as coordinator for. It could still efficiently respond to dispatch requests from other coordinators because the originator will remain in memory.
	targetActiveSequencersLimit   int // Max number of sequencers this node aims to retain in memory concurrently. Hitting this limit will cause an attempt to remove the lowest priority sequencer from memory, and hence require it to be recreated from persisted state if it is needed in the future
}

// Init implements Engine.
func (sMgr *sequencerManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	log.L(sMgr.ctx).Infof("PreInit distributed sequencer manager")
	sMgr.metrics = metrics.InitMetrics(sMgr.ctx, c.MetricsManager().Registry())

	return &components.ManagerInitResult{
		PreCommitHandler: func(ctx context.Context, dbTX persistence.DBTX, blocks []*pldapi.IndexedBlock, transactions []*blockindexer.IndexedTransactionNotify) error {
			latestBlockNumber := blocks[len(blocks)-1].Number
			dbTX.AddPostCommit(func(ctx context.Context) {
				sMgr.OnNewBlockHeight(ctx, latestBlockNumber)
			})
			return nil
		},
	}, nil
}

func (sMgr *sequencerManager) PostInit(c components.AllComponents) error {
	log.L(sMgr.ctx).Infof("PostInit distributed sequencer manager")
	sMgr.components = c
	sMgr.nodeName = sMgr.components.TransportManager().LocalNodeName()
	sMgr.syncPoints = syncpoints.NewSyncPoints(sMgr.ctx, &sMgr.config.Writer, c.Persistence(), c.TxManager(), c.PublicTxManager(), c.TransportManager())
	return nil
}

func (sMgr *sequencerManager) Start() error {
	log.L(sMgr.ctx).Infof("Starting distributed sequencer manager")
	sMgr.syncPoints.Start()
	sMgr.pollForIncompleteTransactions(sMgr.ctx, confutil.DurationMinIfPositive(sMgr.config.TransactionResumePollInterval, pldconf.SequencerMinimum.TransactionResumePollInterval, *pldconf.SequencerDefaults.TransactionResumePollInterval))

	return nil
}

func (sMgr *sequencerManager) Stop() {
	log.L(sMgr.ctx).Infof("Stopping distributed sequencer manager")
	sMgr.StopAllSequencers(sMgr.ctx)
	log.L(sMgr.ctx).Infof("Stopped all sequencers")
	sMgr.syncPoints.Close()
	sMgr.cancelCtx()
}

func NewDistributedSequencerManager(ctx context.Context, config *pldconf.SequencerConfig) components.SequencerManager {

	dsmCtx, dsmCtxCancel := context.WithCancel(log.WithComponent(ctx, "sequencer_manager"))
	sMgr := &sequencerManager{
		ctx:                           dsmCtx,
		cancelCtx:                     dsmCtxCancel,
		config:                        config,
		sequencers:                    make(map[string]*sequencer),
		targetActiveCoordinatorsLimit: confutil.IntMin(config.TargetActiveCoordinators, pldconf.SequencerMinimum.TargetActiveCoordinators, *pldconf.SequencerDefaults.TargetActiveCoordinators),
		targetActiveSequencersLimit:   confutil.IntMin(config.TargetActiveSequencers, pldconf.SequencerMinimum.TargetActiveSequencers, *pldconf.SequencerDefaults.TargetActiveSequencers),
	}
	return sMgr
}

// We may have in-flight transactions that never completed. Load any we have pending and and resume them
func (sMgr *sequencerManager) pollForIncompleteTransactions(ctx context.Context, rePollInterval time.Duration) {
	if rePollInterval <= 0 {
		log.L(ctx).Warnf("Sequencer transaction resume disabled")
		return
	}
	// Repeat getting pending transactions until none are returned. Run in a goroutine to avoid blocking the main thread
	go func() {
	waitForIndexerReady:
		for {
			// On startup we can't assemble any transactions without having a confirmed block height so
			// wait until the indexer is ready
			_, err := sMgr.components.BlockIndexer().GetConfirmedBlockHeight(ctx)
			if err == nil {
				break
			}

			timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
			defer cancel()

			// Wait for the block indexer to be ready
			select {
			case <-timeoutCtx.Done():
				log.L(ctx).Debugf("timeout - check again if indexer is ready")
				break waitForIndexerReady
			case <-ctx.Done():
				log.L(ctx).Errorf("context cancelled - ending DB poll")
				return
			}
		}

		for {
			pageSize := confutil.IntMin(sMgr.config.TransactionResumePageSize, pldconf.SequencerMinimum.TransactionResumePageSize, *pldconf.SequencerDefaults.TransactionResumePageSize)
			maxTransactions := *pldconf.SequencerDefaults.TransactionResumeMaxTransactions
			if sMgr.config.TransactionResumeMaxTransactions != nil {
				maxTransactions = *sMgr.config.TransactionResumeMaxTransactions
			}

			resumedTransactions := 0
			var lastCreatedTime int64

			// Originators are responsible for resuming and re-delegating their own transactions.
			// Paginate through all pending transactions with configurable page size and optional upper limit.
			for maxTransactions > 0 && resumedTransactions < maxTransactions {
				limit := pageSize
				if resumedTransactions+limit > maxTransactions {
					limit = maxTransactions - resumedTransactions
				}

				query := query.NewQueryBuilder().
					Limit(limit).
					Sort("created")
				if lastCreatedTime > 0 {
					log.L(sMgr.ctx).Debugf("Retrieving the next %d incomplete transactions to resume from timestamp %d", limit, lastCreatedTime)
					query.GreaterThan("created", lastCreatedTime)
				} else {
					log.L(sMgr.ctx).Debugf("Retrieving the next %d incomplete transactions to resume", limit)
				}
				q := query.Query()

				pendingTx, err := sMgr.components.TxManager().QueryTransactionsResolved(sMgr.ctx, q, sMgr.components.Persistence().NOTX(), true)
				if err != nil {
					log.L(sMgr.ctx).Errorf("Error querying pending transactions to resume incomplete ones: %s", err)
					break
				}

				resumedTransactions += len(pendingTx)
				log.L(sMgr.ctx).Tracef("Resuming %d transactions", len(pendingTx))
				for _, tx := range pendingTx {
					err = sMgr.HandleTxResume(sMgr.ctx, &components.ValidatedTransaction{
						ResolvedTransaction: *tx,
					})
					if err != nil {
						log.L(sMgr.ctx).Errorf("Error resuming pending transaction %s: %s", tx.Transaction.ID, err)
					}
				}
				if len(pendingTx) > 0 {
					lastCreatedTime = int64(pendingTx[len(pendingTx)-1].Transaction.Created)
				}
				if len(pendingTx) < pageSize {
					break
				}
			}

			// Repeat DB poll every N minutes to check for incomplete transactions to resume
			timeoutCtx, cancel := context.WithTimeout(sMgr.ctx, rePollInterval)
			defer cancel()

			select {
			case <-timeoutCtx.Done():
				log.L(sMgr.ctx).Debug("timeout - checking for pending DB transactions")
			case <-ctx.Done():
				log.L(sMgr.ctx).Debug("context cancelled - ending DB poll")
				return
			}
		}
	}()
}

// Synchronous function to submit a deployment request which is asynchronously processed
// Private transaction manager will receive a notification when the public transaction is confirmed
// (same as for invokes)
func (sMgr *sequencerManager) handleDeployTx(ctx context.Context, tx *components.PrivateContractDeploy) error {
	log.L(ctx).Debugf("handling new private contract deploy transaction: %v", tx)
	if tx.Domain == "" {
		return i18n.NewError(ctx, msgs.MsgSequencerDomainNotProvided)
	}

	domain, err := sMgr.components.DomainManager().GetDomainByName(ctx, tx.Domain)
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainNotFound, tx.Domain)
	}

	err = domain.InitDeploy(ctx, tx)
	if err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgSequencerDeployInitFailed)
	}

	// this is a transaction that will confirm just like invoke transactions
	// unlike invoke transactions, we don't yet have the sequencer thread to dispatch to so we start a new go routine for each deployment
	// TODO - should have a pool of deployment threads? Maybe size of pool should be one? Or at least one per domain?
	sMgr.metrics.IncDispatchedTransactions()
	go sMgr.deploymentLoop(log.WithLogField(sMgr.ctx, "role", "deploy-loop"), domain, tx)

	return nil
}

func (sMgr *sequencerManager) deploymentLoop(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) {
	log.L(ctx).Info("starting deployment loop")

	var err error

	// Resolve keys synchronously on this go routine so that we can return an error if any key resolution fails
	tx.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.RequiredVerifiers))
	for i, v := range tx.RequiredVerifiers {
		// TODO: This is a synchronous cross-node exchange, done sequentially for each verifier.
		// Potentially needs to move to an event-driven model like on invocation.
		verifier, resolveErr := sMgr.components.IdentityResolver().ResolveVerifier(ctx, v.Lookup, v.Algorithm, v.VerifierType)
		if resolveErr != nil {
			err = i18n.WrapError(ctx, resolveErr, msgs.MsgSequencerKeyResolutionFailed, v.Lookup, v.Algorithm, v.VerifierType)
			break
		}
		tx.Verifiers[i] = &prototk.ResolvedVerifier{
			Lookup:       v.Lookup,
			Algorithm:    v.Algorithm,
			Verifier:     verifier,
			VerifierType: v.VerifierType,
		}
	}

	if err == nil {
		err = sMgr.evaluateDeployment(ctx, domain, tx)
	}
	if err != nil {
		log.L(ctx).Errorf("error evaluating deployment: %s", err)
		return
	}

	log.L(ctx).Info("deployment completed successfully")
}

func (sMgr *sequencerManager) evaluateDeployment(ctx context.Context, domain components.Domain, tx *components.PrivateContractDeploy) error {

	// TODO there is a lot of common code between this and the Dispatch function in the sequencer. should really move some of it into a common place
	// and use that as an opportunity to refactor to be more readable

	err := domain.PrepareDeploy(ctx, tx)
	if err != nil {
		return sMgr.revertDeploy(ctx, tx, err)
	}

	publicTransactionEngine := sMgr.components.PublicTxManager()

	// The signer needs to be in our local node or it's an error
	identifier, node, err := pldtypes.PrivateIdentityLocator(tx.Signer).Validate(ctx, sMgr.nodeName, true)
	if err != nil {
		return err
	}
	if node != sMgr.nodeName {
		return i18n.NewError(ctx, msgs.MsgSequencerNonLocalSigningAddr, tx.Signer)
	}

	keyMgr := sMgr.components.KeyManager()
	resolvedAddrs, err := keyMgr.ResolveEthAddressBatchNewDatabaseTX(ctx, []string{identifier})
	if err != nil {
		return sMgr.revertDeploy(ctx, tx, err)
	}

	publicTXs := []*components.PublicTxSubmission{
		{
			Bindings: []*components.PaladinTXReference{{TransactionID: tx.ID, TransactionType: pldapi.TransactionTypePrivate.Enum()}},
			PublicTxInput: pldapi.PublicTxInput{
				From:            resolvedAddrs[0],
				PublicTxOptions: pldapi.PublicTxOptions{}, // TODO: Consider propagation from paladin transaction input
			},
		},
	}

	if tx.InvokeTransaction != nil {
		log.L(ctx).Debug("deploying by invoking a base ledger contract")

		data, err := tx.InvokeTransaction.FunctionABI.EncodeCallDataCtx(ctx, tx.InvokeTransaction.Inputs)
		if err != nil {
			return sMgr.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgSequencerEncodeCallDataFailed))
		}
		publicTXs[0].Data = pldtypes.HexBytes(data)
		publicTXs[0].To = &tx.InvokeTransaction.To

	} else if tx.DeployTransaction != nil {
		// TODO
		return sMgr.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "deployTransaction not implemented"))
	} else {
		return sMgr.revertDeploy(ctx, tx, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "neither InvokeTransaction nor DeployTransaction set"))
	}

	for _, pubTx := range publicTXs {
		err := publicTransactionEngine.ValidateTransaction(ctx, sMgr.components.Persistence().NOTX(), pubTx)
		if err != nil {
			return sMgr.revertDeploy(ctx, tx, i18n.WrapError(ctx, err, msgs.MsgSequencerInternalError, "PrepareSubmissionBatch failed"))
		}
	}

	//transactions are always dispatched as a sequence, even if only a sequence of one
	sequence := &syncpoints.PublicDispatch{
		PrivateTransactionDispatches: []*syncpoints.DispatchPersisted{
			{
				PrivateTransactionID: tx.ID.String(),
			},
		},
	}
	sequence.PublicTxs = publicTXs
	dispatchBatch := &syncpoints.DispatchBatch{
		PublicDispatches: []*syncpoints.PublicDispatch{
			sequence,
		},
	}

	// as this is a deploy we specify the null address
	err = sMgr.syncPoints.PersistDeployDispatchBatch(ctx, dispatchBatch)
	if err != nil {
		log.L(ctx).Errorf("error persisting batch: %s", err)
		return sMgr.revertDeploy(ctx, tx, err)
	}

	return nil
}

func (sMgr *sequencerManager) revertDeploy(ctx context.Context, tx *components.PrivateContractDeploy, err error) error {
	deployError := i18n.WrapError(ctx, err, msgs.MsgSequencerDeployError)

	var tryFinalize func()
	tryFinalize = func() {
		sMgr.syncPoints.QueueTransactionFinalize(ctx, &syncpoints.TransactionFinalizeRequest{
			Domain:          tx.Domain,
			ContractAddress: pldtypes.EthAddress{},
			Originator:      tx.From,
			TransactionID:   tx.ID,
			FailureMessage:  deployError.Error(),
		},
			func(ctx context.Context) {
				log.L(ctx).Debugf("finalized deployment transaction: %s", tx.ID)
			},
			func(ctx context.Context, err error) {
				log.L(ctx).Errorf("error finalizing deployment: %s", err)
				tryFinalize()
			})
	}
	tryFinalize()
	return deployError
}

// Handling a new transaction. We don't need to persist anything under the DBTX but we do need to ensure the DBTX
// has committed before passing any events to the sequencer to process the tranasction.
func (sMgr *sequencerManager) HandleNewTx(ctx context.Context, dbTX persistence.DBTX, txi *components.ValidatedTransaction) error {
	tx := txi.Transaction

	// First check if the TX has incomplete or failed dependencies
	blockedByDependencies, err := sMgr.components.TxManager().BlockedByDependencies(ctx, dbTX, txi)
	if err != nil {
		return err
	}
	if blockedByDependencies {
		// There are 2 ways this TX will be resumed given that it has incomplete dependencies:
		// 1. The periodic sequencer poll loop will attempt to resume it, making this same check
		// 2. The dependency listener will see a receipt and tap the sequencer manager to check if dependents can be processed
		return nil
	}

	if tx.To == nil {
		if txi.Transaction.SubmitMode.V() != pldapi.SubmitModeAuto {
			return i18n.NewError(ctx, msgs.MsgSequencerPrepareNotSupportedDeploy)
		}
		log.L(sMgr.ctx).Infof("handling deploy transaction %s from signer %s", tx.ID, tx.From)
		return sMgr.handleDeployTx(ctx, &components.PrivateContractDeploy{
			ID:     *tx.ID,
			Domain: tx.Domain,
			From:   tx.From,
			Inputs: tx.Data,
		})
	}
	intent := prototk.TransactionSpecification_SEND_TRANSACTION
	if txi.Transaction.SubmitMode.V() == pldapi.SubmitModeExternal {
		intent = prototk.TransactionSpecification_PREPARE_TRANSACTION
	}
	if txi.Function == nil || txi.Function.Definition == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerFunctionNotProvided)
	}
	log.L(sMgr.ctx).Infof("handling transaction %s from signer %s", tx.ID, tx.From)
	return sMgr.handleTx(ctx, dbTX, &components.PrivateTransaction{
		ID:      *tx.ID,
		Domain:  tx.Domain,
		Address: *tx.To,
		Intent:  intent,
	}, &txi.ResolvedTransaction, false)
}

// Resume a transaction we have read from the DB on startup.
func (sMgr *sequencerManager) HandleTxResume(ctx context.Context, txi *components.ValidatedTransaction) error {
	return sMgr.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		tx := txi.Transaction

		// First check if the TX has incomplete or failed dependencies
		blockedByDependencies, err := sMgr.components.TxManager().BlockedByDependencies(ctx, dbTX, txi)
		if err != nil {
			return err
		}
		if blockedByDependencies {
			// There are 2 ways this TX will be resumed given that it has incomplete dependencies:
			// 1. The periodic sequencer poll loop will attempt to resume it, calling us again at which point we will make this same check
			// 2. The dependency listener will see a receipt and tap the sequencer manager to check if dependents can be processed
			return nil
		}

		if tx.To == nil {
			if txi.Transaction.SubmitMode.V() != pldapi.SubmitModeAuto {
				return i18n.NewError(ctx, msgs.MsgSequencerPrepareNotSupportedDeploy)
			}
			log.L(sMgr.ctx).Infof("resuming deploy transaction %s from %s", txi.Transaction.ID, txi.Transaction.From)
			return sMgr.handleDeployTx(ctx, &components.PrivateContractDeploy{
				ID:     *tx.ID,
				Domain: tx.Domain,
				From:   tx.From,
				Inputs: tx.Data,
			})
		}
		intent := prototk.TransactionSpecification_SEND_TRANSACTION
		if txi.Transaction.SubmitMode.V() == pldapi.SubmitModeExternal {
			intent = prototk.TransactionSpecification_PREPARE_TRANSACTION
		}
		if txi.Function == nil || txi.Function.Definition == nil {
			return i18n.NewError(ctx, msgs.MsgSequencerFunctionNotProvided)
		}
		log.L(sMgr.ctx).Infof("resuming transaction %s from %s", tx.ID, tx.From)
		return sMgr.handleTx(ctx, dbTX, &components.PrivateTransaction{
			ID:      *tx.ID,
			Domain:  tx.Domain,
			Address: *tx.To,
			Intent:  intent,
		}, &txi.ResolvedTransaction, true)
	})
}

// Start processing a new or resumed transaction. The state machine is designed to be idempotent to new transactions with the same ID being resumed, so there is no checking
// in this function that the transaction isn't already being processed by the state machine.
func (sMgr *sequencerManager) handleTx(ctx context.Context, dbTX persistence.DBTX, tx *components.PrivateTransaction, localTx *components.ResolvedTransaction, resume bool) error {
	contractAddr := *localTx.Transaction.To
	emptyAddress := pldtypes.EthAddress{}
	if contractAddr == emptyAddress {
		return i18n.NewError(ctx, msgs.MsgSequencerContractAddressNotProvided)
	}

	domainAPI, err := sMgr.components.DomainManager().GetSmartContractByAddress(ctx, dbTX, contractAddr)
	if err != nil {
		return err
	}

	domainName := domainAPI.Domain().Name()
	if localTx.Transaction.Domain != "" && domainName != localTx.Transaction.Domain {
		return i18n.NewError(ctx, msgs.MsgSequencerDomainMismatch, localTx.Transaction.Domain, domainName, domainAPI.Address())
	}
	localTx.Transaction.Domain = domainName

	err = domainAPI.InitTransaction(ctx, tx, localTx)
	if err != nil {
		return err
	}

	if tx.PreAssembly == nil {
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "PreAssembly is nil")
	}

	sequencer, err := sMgr.LoadSequencer(ctx, dbTX, contractAddr, domainAPI, tx)
	if err != nil {
		return err
	}

	txCreatedEvent := &originator.TransactionCreatedEvent{
		Transaction: tx,
	}

	if !resume {
		dbTX.AddPostCommit(func(ctx context.Context) {
			sequencer.GetOriginator().QueueEvent(ctx, txCreatedEvent)
			sMgr.metrics.IncAcceptedTransactions()
		})
	} else {
		// We're resuming an existing transaction, no need for a post-commit, just handle the TX
		sequencer.GetOriginator().QueueEvent(ctx, txCreatedEvent)
	}

	return nil
}

func (sMgr *sequencerManager) OnNewBlockHeight(ctx context.Context, blockHeight int64) {
	log.L(ctx).Tracef("new block height %d", blockHeight)
	sMgr.blockHeightMutex.Lock()
	defer sMgr.blockHeightMutex.Unlock()
	sMgr.blockHeight = blockHeight
}

func (sMgr *sequencerManager) GetBlockHeight() int64 {
	sMgr.blockHeightMutex.RLock()
	defer sMgr.blockHeightMutex.RUnlock()
	return sMgr.blockHeight
}

func (sMgr *sequencerManager) GetNodeName() string {
	return sMgr.nodeName
}

func (sMgr *sequencerManager) GetTxStatus(ctx context.Context, domainAddress string, txID uuid.UUID) (status components.PrivateTxStatus, err error) {
	sequencer, err := sMgr.LoadSequencer(ctx, sMgr.components.Persistence().NOTX(), *pldtypes.MustEthAddress(domainAddress), nil, nil)
	if err != nil || sequencer == nil {
		return components.PrivateTxStatus{
			TxID:   txID.String(),
			Status: "unknown",
		}, err
	}
	return sequencer.GetOriginator().GetTxStatus(ctx, txID)
}

func (sMgr *sequencerManager) HandleTransactionCollected(ctx context.Context, signerAddress string, contractAddress string, txID uuid.UUID) error {
	log.L(sMgr.ctx).Tracef("HandleTransactionCollected %s %s %s", signerAddress, contractAddress, txID.String())

	// Get the sequencer for the signer address
	sequencer, err := sMgr.GetSequencer(ctx, *pldtypes.MustEthAddress(contractAddress))
	if err != nil {
		return err
	}

	// Public TX manager doesn't distinguish between new contracts (for which a sequencer doesn't yet exist) and a transaction,
	// so accept the fact that there may not be a sequencer for this public TX submission
	if sequencer != nil {
		collectedEvent := &coordinatorTx.CollectedEvent{
			BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
				TransactionID: txID,
			},
			SignerAddress: *pldtypes.MustEthAddress(signerAddress),
		}

		// Public TX manager events are informational rather than critical for the coordinator. This function is called as part of
		// orchestrator polling so it is critical we do not block here waiting on a full event queue.
		// TODO - return to the idea of substates for these
		sequencer.GetCoordinator().TryQueueEvent(ctx, collectedEvent)
	}

	return nil
}

// Handle nonce assignment by updating our coordination state machine(s)
func (sMgr *sequencerManager) HandleNonceAssigned(ctx context.Context, nonce uint64, contractAddress string, txID uuid.UUID) error {
	log.L(sMgr.ctx).Tracef("HandleNonceAssigned %d %s %s", nonce, contractAddress, txID.String())

	// Get the sequencer for the signer address
	sequencer, err := sMgr.GetSequencer(ctx, *pldtypes.MustEthAddress(contractAddress))
	if err != nil {
		return err
	}

	// Public TX manager doesn't distinguish between new contracts (for which a sequencer doesn't yet exist) and a transaction,
	// so accept the fact that there may not be a sequencer for this public TX submission
	if sequencer != nil {
		coordinatorNonceAllocatedEvent := &coordinatorTx.NonceAllocatedEvent{
			BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
				TransactionID: txID,
			},
			Nonce: nonce,
		}
		// Public TX manager events are informational rather than critical for the coordinator. This function is called as part of
		// orchestrator polling so it is critical we do not block here waiting on a full event queue.
		sequencer.GetCoordinator().TryQueueEvent(ctx, coordinatorNonceAllocatedEvent)
	}

	return nil
}

// Handle public TX submission, both for our own coordination state machine(s), and by distributing this public TX submission to other parties who need to have it
func (sMgr *sequencerManager) HandlePublicTXSubmission(ctx context.Context, dbTX persistence.DBTX, txID uuid.UUID, tx *pldapi.PublicTxWithBinding) error {
	log.L(sMgr.ctx).Debugf("HandlePublicTXSubmission TXID %s", txID.String())

	deploy := tx.To == nil
	if !deploy {
		sequencer, err := sMgr.GetSequencer(ctx, *pldtypes.MustEthAddress(tx.TransactionContractAddress))
		if err != nil {
			return err
		}

		// Public TX manager doesn't distinguish between new contracts (for which a sequencer doesn't yet exist) and a transaction,
		// so accept the fact that there may not be a sequencer for this public TX submission
		if sequencer != nil {
			coordinatorSubmittedEvent := &coordinatorTx.SubmittedEvent{
				BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
					TransactionID: txID,
				},
				SubmissionHash: *tx.TransactionHash,
			}
			// Public TX manager events are informational rather than critical for the coordinator. This function is called as part of
			// the public tx manager submission writer so it is critical we do not block here waiting on a full event queue.
			sequencer.GetCoordinator().TryQueueEvent(ctx, coordinatorSubmittedEvent)
			// The coordinator transaction state machine sends TransactionSubmitted to the originator when it processes this event
		}

		// As well as updating ths state machine(s) we must distribute the public TX submission to the originator who needs visibility of public transactions
		// related to their coordinated private transaction submissions
		senderNode, err := pldtypes.PrivateIdentityLocator(tx.TransactionSender).Node(ctx, false)
		if err != nil {
			return err
		}
		if senderNode != sMgr.nodeName {
			log.L(ctx).Debugf("Distributing public transaction submission to node %s", senderNode)
			// Send reliable message to the node under the current DBTX
			err = sMgr.components.TransportManager().SendReliable(ctx, dbTX, &pldapi.ReliableMessage{
				MessageType: pldapi.RMTPublicTransactionSubmission.Enum(),
				Metadata:    pldtypes.JSONString(tx),
				Node:        senderNode,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (sMgr *sequencerManager) handleTransactionConfirmedSuccess(ctx context.Context, confirmedTxn *components.TxCompletion, nonce *pldtypes.HexUint64) error {
	log.L(sMgr.ctx).Tracef("handleTransactionConfirmedSuccess %s nonce=%v", confirmedTxn.TransactionID.String(), nonce)
	sMgr.metrics.IncConfirmedTransactions()

	// A transaction can be confirmed after the coordinating node has restarted. The coordinator doesn't persist the private TX, it relies
	// on the originating node to delegate the private TX to it. handleTransactionConfirmedSuccess first checks if a public TX for that request has been confirmed
	// on chain, so in this context we will assume we have the private TX in memory from which we can determine the originating node for confirmation events.

	// For a deploy we won't have tracked the transaction through the state machine
	if confirmedTxn.ContractAddress != nil {
		return nil
	}

	// Invoke of an existing contract.
	contractAddress := confirmedTxn.PSC.Address()

	sequencer, err := sMgr.GetSequencer(ctx, contractAddress)
	if err != nil {
		return err
	}

	// If we don't have a loaded sequencer already then a newly loaded one will not know about this transaction.
	if sequencer == nil {
		return nil
	}

	// we leave it to the coordinator to decide whether it is in a state where it handles the event
	// and check whether it's a transaction that it is tracking
	// the transaction was successful
	sequencer.GetCoordinator().QueueEvent(ctx, &coordinatorTx.ConfirmedSuccessEvent{
		BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: confirmedTxn.TransactionID,
		},
		Hash:  confirmedTxn.OnChain.TransactionHash,
		Nonce: nonce,
	})
	return nil
}

func (sMgr *sequencerManager) queueConfirmedRevertedEventToCoordinator(ctx context.Context, contractAddress pldtypes.EthAddress, txID uuid.UUID, revertData pldtypes.HexBytes, onChain pldtypes.OnChainLocation, nonce *pldtypes.HexUint64) error {
	sequencer, err := sMgr.GetSequencer(ctx, contractAddress)
	if err != nil {
		return err
	}
	// If we don't have a loaded sequencer already then a newly loaded one will not know about this transaction
	if sequencer == nil {
		return nil
	}

	// we leave it to the coordinator to decide whether it is in a state where it handles the event
	// and check whether it's a transaction that it is tracking

	sequencer.GetCoordinator().QueueEvent(ctx, &coordinatorTx.ConfirmedRevertedEvent{
		BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
			BaseEvent: common.BaseEvent{
				EventTime: time.Now(),
			},
			TransactionID: txID,
		},
		Hash:         onChain.TransactionHash,
		RevertReason: revertData,
		OnChain:      onChain,
		Nonce:        nonce,
	})
	return nil
}

func (sMgr *sequencerManager) HandleChainedTransactionOutcome(ctx context.Context, contractAddress pldtypes.EthAddress, txID uuid.UUID, receiptType components.ReceiptType, failureMessage string, revertData pldtypes.HexBytes, onChain pldtypes.OnChainLocation) {
	log.L(ctx).Infof("HandleChainedTransactionOutcome txID=%s contract=%s receiptType=%d", txID, contractAddress, receiptType)

	sequencer, err := sMgr.GetSequencer(ctx, contractAddress)
	if err != nil {
		log.L(ctx).Errorf("HandleChainedTransactionOutcome: error getting sequencer for %s: %s", contractAddress, err)
		return
	}
	if sequencer == nil {
		log.L(ctx).Warnf("HandleChainedTransactionOutcome: no loaded sequencer for contract %s txID=%s", contractAddress, txID)
		return
	}

	switch receiptType {
	case components.RT_Success:
		log.L(ctx).Infof("HandleChainedTransactionOutcome: queuing ConfirmedSuccessEvent for parent txID=%s on contract=%s", txID, contractAddress)
		sequencer.GetCoordinator().QueueEvent(ctx, &coordinatorTx.ConfirmedSuccessEvent{
			BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
				BaseEvent: common.BaseEvent{
					EventTime: time.Now(),
				},
				TransactionID: txID,
			},
		})
	case components.RT_FailedOnChainWithRevertData:
		log.L(ctx).Infof("HandleChainedTransactionOutcome: queuing ConfirmedRevertedEvent (on-chain) for parent txID=%s on contract=%s hasRevertData=%t", txID, contractAddress, len(revertData) > 0)
		sequencer.GetCoordinator().QueueEvent(ctx, &coordinatorTx.ConfirmedRevertedEvent{
			BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
				BaseEvent: common.BaseEvent{
					EventTime: time.Now(),
				},
				TransactionID: txID,
			},
			RevertReason: revertData,
			OnChain:      onChain,
		})
	case components.RT_FailedWithMessage:
		log.L(ctx).Infof("HandleChainedTransactionOutcome: queuing ConfirmedRevertedEvent (off-chain) for parent txID=%s on contract=%s", txID, contractAddress)
		sequencer.GetCoordinator().QueueEvent(ctx, &coordinatorTx.ConfirmedRevertedEvent{
			BaseCoordinatorEvent: coordinatorTx.BaseCoordinatorEvent{
				BaseEvent: common.BaseEvent{
					EventTime: time.Now(),
				},
				TransactionID: txID,
			},
			FailureMessage: failureMessage,
		})
	default:
		log.L(ctx).Errorf("HandleChainedTransactionOutcome: unexpected receipt type %d for txID=%s", receiptType, txID)
	}
}

func (sMgr *sequencerManager) HandleDirectTransactionRevert(ctx context.Context, dbTX persistence.DBTX, failures []*components.PublicTxMatch) error {
	log.L(sMgr.ctx).Tracef("HandleDirectTransactionRevert %d", len(failures))
	sMgr.metrics.IncRevertedTransactions()
	_ = dbTX

	for _, tx := range failures {
		contractAddress := tx.To

		if tx.From == nil {
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, "nil From address for confirmed transaction %s", tx.TransactionID)
		}

		nonceVal := pldtypes.HexUint64(tx.Nonce)
		onChain := pldtypes.OnChainLocation{
			Type:             pldtypes.OnChainTransaction,
			TransactionHash:  tx.Hash,
			BlockNumber:      tx.BlockNumber,
			TransactionIndex: tx.TransactionIndex,
		}
		err := sMgr.queueConfirmedRevertedEventToCoordinator(ctx, *contractAddress, tx.TransactionID, tx.RevertReason, onChain, &nonceVal)
		if err != nil {
			return err
		}
	}
	return nil
}

func (sMgr *sequencerManager) BuildNullifiers(ctx context.Context, stateDistributions []*components.StateDistributionWithData) (nullifiers []*components.NullifierUpsert, err error) {

	nullifiers = []*components.NullifierUpsert{}
	err = sMgr.components.Persistence().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		for _, s := range stateDistributions {
			if s.NullifierAlgorithm == nil || s.NullifierVerifierType == nil || s.NullifierPayloadType == nil {
				log.L(ctx).Debugf("no nullifier required for state %s on node %s", s.StateID, sMgr.nodeName)
				continue
			}

			nullifier, err := sMgr.BuildNullifier(ctx, sMgr.components.KeyManager().KeyResolverForDBTX(dbTX), s)
			if err != nil {
				return err
			}

			nullifiers = append(nullifiers, nullifier)
		}
		return nil
	})
	return nullifiers, err
}

func (sMgr *sequencerManager) BuildNullifier(ctx context.Context, kr components.KeyResolver, s *components.StateDistributionWithData) (*components.NullifierUpsert, error) {
	// We need to call the signing engine with the local identity to build the nullifier
	log.L(ctx).Debugf("generating nullifier for state %s on node %s (algorithm=%s,verifierType=%s,payloadType=%s)",
		s.StateID, sMgr.nodeName, *s.NullifierAlgorithm, *s.NullifierVerifierType, *s.NullifierPayloadType)

	// We require a fully qualified identifier for the local node in this function
	identifier, node, err := pldtypes.PrivateIdentityLocator(s.IdentityLocator).Validate(ctx, "", false)
	if err != nil || node != sMgr.nodeName {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierNotLocal)
	}

	// Call the signing engine to build the nullifier
	var nulliferBytes []byte
	mapping, err := kr.ResolveKey(ctx, identifier, *s.NullifierAlgorithm, *s.NullifierVerifierType)
	if err == nil {
		nulliferBytes, err = sMgr.components.KeyManager().Sign(ctx, mapping, *s.NullifierPayloadType, s.StateData.Bytes())
	}
	if err != nil || len(nulliferBytes) == 0 {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierFail, s.StateID)
	}
	return &components.NullifierUpsert{
		ID:    nulliferBytes,
		State: pldtypes.MustParseHexBytes(s.StateID),
	}, nil
}

func (sMgr *sequencerManager) CallPrivateSmartContract(ctx context.Context, call *components.ResolvedTransaction) (*abi.ComponentValue, error) {

	callTx := call.Transaction
	psc, err := sMgr.components.DomainManager().GetSmartContractByAddress(ctx, sMgr.components.Persistence().NOTX(), *callTx.To)
	if err != nil {
		return nil, err
	}

	domainName := psc.Domain().Name()
	if callTx.Domain != "" && domainName != callTx.Domain {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerDomainMismatch, callTx.Domain, domainName, psc.Address())
	}
	callTx.Domain = domainName

	// Initialize the call, returning at list of required verifiers
	requiredVerifiers, err := psc.InitCall(ctx, call)
	if err != nil {
		return nil, err
	}

	// Do the verification in-line and synchronously for call (there is caching in the identity resolver)
	identityResolver := sMgr.components.IdentityResolver()
	verifiers := make([]*prototk.ResolvedVerifier, len(requiredVerifiers))
	for i, r := range requiredVerifiers {
		verifier, err := identityResolver.ResolveVerifier(ctx, r.Lookup, r.Algorithm, r.VerifierType)
		if err != nil {
			return nil, err
		}
		verifiers[i] = &prototk.ResolvedVerifier{
			Lookup:       r.Lookup,
			Algorithm:    r.Algorithm,
			VerifierType: r.VerifierType,
			Verifier:     verifier,
		}
	}

	// Create a throwaway domain context for this call
	dCtx := sMgr.components.StateManager().NewDomainContext(ctx, psc.Domain(), psc.Address())
	defer dCtx.Close()

	// Do the actual call
	return psc.ExecCall(dCtx, sMgr.components.Persistence().NOTX(), call, verifiers)
}

func (sMgr *sequencerManager) BuildStateDistributions(ctx context.Context, tx *components.PrivateTransaction) (*components.StateDistributionSet, error) {
	return common.NewStateDistributionBuilder(sMgr.nodeName, tx).Build(ctx)
}

// PrivateTransactionsConfirmed processes a pre-sorted batch of completions synchronously.
// It is expected to be called from a per-domain worker goroutine so that ordering
// within a domain's event stream is preserved.
func (sMgr *sequencerManager) PrivateTransactionsConfirmed(ctx context.Context, completions []*components.TxCompletion) {
	persistence := sMgr.components.Persistence()
	publicTxManager := sMgr.components.PublicTxManager()

	for _, completion := range completions {
		pubBindingTx, err := publicTxManager.QueryPublicTxForTransactions(ctx, persistence.NOTX(), []uuid.UUID{completion.TransactionID}, nil)
		if err != nil {
			log.L(ctx).Errorf("Error getting public transaction by ID: %s", err)
			continue
		}

		confirmedWithPublicTX := false

		for _, pubTx := range pubBindingTx {
			for _, publicTx := range pubTx {
				log.L(ctx).Debugf("Checking public transactions for TX ID %s to find a match for the receipt we are processing %s", completion.TransactionID, publicTx.TransactionHash)
				if publicTx.TransactionHash.Equals(&completion.OnChain.TransactionHash) {
					confirmedWithPublicTX = true
					log.L(ctx).Debugf("Found a match for the receipt we are processing %s", publicTx.TransactionHash)
					err = sMgr.handleTransactionConfirmedSuccess(ctx, completion, publicTx.Nonce)
					if err != nil {
						// Log but continue confirming other transactions
						log.L(ctx).Errorf("Error handling transaction confirmed event: %s", err)
					}
				}
			}
		}

		// For private transactions that are being confirmed by virtue of a successful chained private transaction, we don't give the distributed sequencer any information
		// about the underlying chained public TX.
		if !confirmedWithPublicTX {
			// Only treat "no public TX match" as chained if this transaction has locally recorded chained children.
			// Otherwise this node is not the relevant coordinator context for this confirmation.
			if completion.ContractAddress == nil {
				var chainedCount int64
				err := persistence.NOTX().DB().
					WithContext(ctx).
					Table("chained_private_txns").
					Where(`"transaction" = ?`, completion.TransactionID).
					Count(&chainedCount).
					Error
				if err != nil {
					log.L(ctx).Errorf("Error checking chained records for transaction %s: %s", completion.TransactionID, err)
					continue
				}
				if chainedCount == 0 {
					log.L(ctx).Debugf("No public TX found for %s and no locally chained transactions recorded; skipping sequencer confirmation", completion.TransactionID)
					continue
				}
			}
			log.L(ctx).Debugf("No public TX found, confirming %s via locally chained transaction", completion.TransactionID)
			err = sMgr.handleTransactionConfirmedSuccess(ctx, completion, nil)
			if err != nil {
				// Log but continue confirming other transactions
				log.L(ctx).Errorf("Error handling transaction confirmed event: %s", err)
			}
		}
	}
}
