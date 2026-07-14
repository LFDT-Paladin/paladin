/*
 * Copyright © 2025 Kaleido, Inc.
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
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type EngineIntegration interface {
	WriteStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error
	MapPotentialStates(ctx context.Context, potentialStates []*prototk.NewState, createdByTX *components.PrivateTransaction) (stateUpserts []*components.StateUpsert, err error)
	GetBlockHeight(ctx context.Context) int64
	// Domain returns the domain associated with the contract being sequenced.
	Domain() components.Domain
	// CheckPendingPrivateStateData returns true when the node has all private state data for
	// opted-in domain contracts up to and including the provided block number.
	CheckPendingPrivateStateData(ctx context.Context, block int64) (bool, error)
	//Assemble and sign is a single, synchronous operation that assembles a transaction using the domain smart contract
	// and then fulfills any signature requests in the attestation plan
	// there would be a benefit in separating this out to `assemble` and `sign` steps and to make then asynchronous
	// In particular, signing could involved collecting multiple signatures and the signing module may be remote
	// and unknown latency could incur back pressure to the state machines input channel
	//However, to fully reap the benefits of tolerating latency in this phase, we would need to revisit the algorithm that currently
	// assumes that the coordinator will not assemble any transactions while it is waiting for a signed post assembly for one transaction
	// . e.g. it might make sense to split out the assembling and gatheringSignatures into separate states on the coordinator side so that it can
	// single thread assembly and still tolerate latency in the signing phase.
	AssembleAndSign(ctx context.Context, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) (*components.TransactionPostAssembly, error)
}

func NewEngineIntegration(ctx context.Context, allComponents components.AllComponents, nodeName string, domainSmartContract components.DomainSmartContract, domainStateWriter components.DomainStateWriter) EngineIntegration {
	return &engineIntegration{
		components:          allComponents,
		domainSmartContract: domainSmartContract,
		domainStateWriter:   domainStateWriter,
		nodeName:            nodeName,
	}

}

type engineIntegration struct {
	components          components.AllComponents
	domainSmartContract components.DomainSmartContract
	domainStateWriter   components.DomainStateWriter
	nodeName            string
}

func (e *engineIntegration) MapPotentialStates(ctx context.Context, potentialStates []*prototk.NewState, createdByTX *components.PrivateTransaction) (stateUpserts []*components.StateUpsert, err error) {
	return e.domainSmartContract.MapPotentialStates(ctx, potentialStates, true, createdByTX)
}

func (e *engineIntegration) WriteStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error {

	if (txn.PostAssembly.OutputStatesPotential != nil && txn.PostAssembly.OutputStates == nil) || (txn.PostAssembly.InfoStatesPotential != nil && txn.PostAssembly.InfoStates == nil) {
		readTX := e.components.Persistence().NOTX() // no DB transaction required here for the reads from the DB (writes happen on syncpoint flusher)
		err := e.domainSmartContract.WritePotentialStates(ctx, e.domainStateWriter, readTX, txn)
		if err != nil {
			// Any error from WritePotentialStates is likely to be caused by an invalid init or assemble of the transaction
			// which is most likely a programming error in the domain or the domain manager or the sequencer
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, err)
		} else {
			log.L(ctx).Debugf("Potential states written for domain=%s", e.domainSmartContract.Domain().Name())
		}
	}

	return nil

}

func (e *engineIntegration) GetBlockHeight(_ context.Context) int64 {
	return e.domainSmartContract.Domain().GetBlockHeight()
}

func (e *engineIntegration) Domain() components.Domain {
	return e.domainSmartContract.Domain()
}

func (e *engineIntegration) CheckPendingPrivateStateData(ctx context.Context, block int64) (bool, error) {
	if !e.domainSmartContract.Domain().FullStateAvailablityRequired() {
		return true, nil
	}
	return e.components.StateManager().CheckPendingPrivateStateDataForContract(
		ctx, e.components.Persistence().NOTX(),
		e.domainSmartContract.Address().String(), block,
	)
}

// assemble a transaction that we are not coordinating, using the provided state locks
// all errors are assumed to be transient and the request should be retried
// if the domain as deemed the request as invalid then it will communicate the `revert` directive via the AssembleTransactionResponse_REVERT result without any error
func (e *engineIntegration) AssembleAndSign(ctx context.Context, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) (*components.TransactionPostAssembly, error) {

	log.L(ctx).Debugf("Assembling transaction %s. Creating domain context with coordinator state locks", transactionID)

	// Create a domain context just for this call that the snapshot can be loaded into.
	dqc := e.components.StateManager().NewDomainQueryContext(ctx, e.domainSmartContract.Domain(), e.domainSmartContract.Address())
	defer dqc.Close(ctx)

	err := dqc.ImportSnapshot(ctx, stateLocksJSON)
	if err != nil {
		return nil, err
	}

	resolvedVerifiers := make([]*prototk.ResolvedVerifier, 0, len(preAssembly.RequiredVerifiers))
	for _, v := range preAssembly.RequiredVerifiers {
		log.L(ctx).Debugf("resolving required verifier %s", v.Lookup)
		verifier, err := e.components.IdentityResolver().ResolveVerifier(
			ctx,
			v.Lookup,
			v.Algorithm,
			v.VerifierType,
		)
		if err != nil {
			return nil, err
		}
		resolvedVerifiers = append(resolvedVerifiers, &prototk.ResolvedVerifier{
			Lookup:       v.Lookup,
			Algorithm:    v.Algorithm,
			VerifierType: v.VerifierType,
			Verifier:     verifier,
		})
	}

	return e.assembleAndSign(ctx, transactionID, preAssembly, resolvedVerifiers, dqc)
}

func (e *engineIntegration) resolveLocalTransaction(ctx context.Context, transactionID uuid.UUID) (*components.ResolvedTransaction, error) {
	locallyResolvedTx, err := e.components.TxManager().GetResolvedTransactionByID(ctx, transactionID)
	if err == nil && locallyResolvedTx == nil {
		err = i18n.WrapError(ctx, err, msgs.MsgSequencerAssembleTxnNotFound, transactionID)
	}
	return locallyResolvedTx, err
}

func (e *engineIntegration) assembleAndSign(ctx context.Context, transactionID uuid.UUID, preAssembly *components.TransactionPreAssembly, resolvedVerifiers []*prototk.ResolvedVerifier, domainQueryContext components.DomainQueryContext) (*components.TransactionPostAssembly, error) {
	localTx, err := e.resolveLocalTransaction(ctx, transactionID)
	if err != nil || localTx.Transaction.Domain != e.domainSmartContract.Domain().Name() || localTx.Transaction.To == nil || *localTx.Transaction.To != e.domainSmartContract.Address() {
		if err == nil {
			log.L(ctx).Errorf("transaction %s for invalid domain/address domain=%s (expected=%s) to=%s (expected=%s)",
				transactionID, localTx.Transaction.Domain, e.domainSmartContract.Domain().Name(), localTx.Transaction.To, e.domainSmartContract.Address())
		}
		err := i18n.WrapError(ctx, err, msgs.MsgSequencerAssembleRequestInvalid, transactionID)
		return nil, err
	}
	transaction := &components.PrivateTransaction{
		ID:          transactionID,
		Domain:      localTx.Transaction.Domain,
		Address:     *localTx.Transaction.To,
		PreAssembly: preAssembly,
	}

	/*
	 * Assemble
	 */
	log.L(ctx).Debugf("Assembling transaction: %+v", transaction)
	err = e.domainSmartContract.AssembleTransaction(ctx, domainQueryContext, e.components.Persistence().NOTX(), transaction, localTx, resolvedVerifiers)
	if err != nil {
		log.L(ctx).Errorf("error assembling transaction: %s", err)
		return nil, err
	}
	if transaction.PostAssembly == nil {
		// This is most likely a programming error in the domain
		err := i18n.NewError(ctx, msgs.MsgSequencerInternalError, "AssembleTransaction returned nil PostAssembly")
		return nil, err
	}

	// Some validation that we are confident we can execute the given attestation plan
	for _, attRequest := range transaction.PostAssembly.AttestationPlan {
		switch attRequest.AttestationType {
		case prototk.AttestationType_ENDORSE:
		case prototk.AttestationType_SIGN:
		default:
			errorMessage := fmt.Sprintf("Unsupported attestation type: %s", attRequest.AttestationType)
			log.L(ctx).Error(errorMessage)
			return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, errorMessage)
		}
	}

	/*
	 * Sign
	 */
	for _, attRequest := range transaction.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_SIGN {
			for _, partyName := range attRequest.Parties {
				log.L(ctx).Debugf("validating identity locator for signing party %s", partyName)
				unqualifiedLookup, signerNode, err := pldtypes.PrivateIdentityLocator(partyName).Validate(ctx, e.nodeName, true)
				if err != nil {
					log.L(ctx).Errorf("failed to validate identity locator for signing party %s: %s", partyName, err)
					return nil, err
				}
				if signerNode == e.nodeName {
					log.L(ctx).Debugf("we are in the signing parties list - signing")

					keyMgr := e.components.KeyManager()
					resolvedKey, err := keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, attRequest.Algorithm, attRequest.VerifierType)
					if err != nil {
						log.L(ctx).Errorf("failed to resolve local signer for %s (algorithm=%s): %s", unqualifiedLookup, attRequest.Algorithm, err)
						return nil, i18n.WrapError(ctx, err, msgs.MsgSequencerResolveError, unqualifiedLookup, attRequest.Algorithm)
					}

					signaturePayload, err := keyMgr.Sign(ctx, resolvedKey, attRequest.PayloadType, attRequest.Payload)
					if err != nil {
						log.L(ctx).Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", unqualifiedLookup, resolvedKey.Verifier.Verifier, attRequest.Algorithm, err)
						return nil, i18n.WrapError(ctx, err, msgs.MsgSequencerSignError, unqualifiedLookup, resolvedKey.Verifier.Verifier, attRequest.Algorithm)
					}
					log.L(ctx).Debugf("payload: %x signed %x by %s (%s)", attRequest.Payload, signaturePayload, unqualifiedLookup, resolvedKey.Verifier.Verifier)

					transaction.PostAssembly.Signatures = append(transaction.PostAssembly.Signatures, &prototk.AttestationResult{
						Name:            attRequest.Name,
						AttestationType: attRequest.AttestationType,
						Verifier: &prototk.ResolvedVerifier{
							Lookup:       partyName,
							Algorithm:    attRequest.Algorithm,
							Verifier:     resolvedKey.Verifier.Verifier,
							VerifierType: attRequest.VerifierType,
						},
						Payload:     signaturePayload,
						PayloadType: &attRequest.PayloadType,
					})
				} else {
					log.L(ctx).Warnf("ignoring sign request of transaction %s for remote party %s ", transactionID, partyName)
				}
			}
		} else {
			log.L(ctx).Debugf("ignoring attestationType %s for fulfillment later", attRequest.AttestationType)
		}
	}

	transaction.PostAssembly.ResolvedVerifiers = resolvedVerifiers

	if log.IsDebugEnabled() {
		stateIDs := ""
		for _, state := range transaction.PostAssembly.OutputStates {
			stateIDs += "," + state.ID.String()
		}
		log.L(ctx).Debugf("Assembled transaction %s, state IDs: %s, result: %s", transactionID, stateIDs, transaction.PostAssembly.AssemblyResult.String())
	}
	return transaction.PostAssembly, nil
}
