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
	ResolveStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error
	GetBlockHeight(ctx context.Context) int64
	// Domain returns the domain associated with the contract being sequenced.
	Domain() components.Domain
	// CheckPendingPrivateStateData returns true when the node has all private state data for
	// opted-in domain contracts up to and including the provided block number.
	CheckPendingPrivateStateData(ctx context.Context, block int64) (bool, error)
	// Assemble assembles a transaction using the domain smart contract, attaches the resolved verifiers,
	// and validates the attestation plan. It does NOT sign: the returned PostAssembly has empty Signatures.
	// Signing is performed separately (and off the coordinator's serialized assembly path) via SignAttestation,
	// so this call carries only the states+verifiers the coordinator needs to release its assembly slot.
	Assemble(ctx context.Context, transactionID uuid.UUID, preAssembly *prototk.TransactionPreAssembly, resolvedVerifiers []*prototk.ResolvedVerifier, stateSnapshot *prototk.StateSnapshot, blockHeight int64) (*prototk.TransactionPostAssembly, error)
	// SignAttestation signs a single SIGN attestation request for the given party using the local key manager.
	// It returns (nil, nil) when the party is not local to this node — remote SIGN parties are not signed here,
	// because only the originating node produces signatures under the push model.
	SignAttestation(ctx context.Context, transactionID uuid.UUID, attRequest *prototk.AttestationRequest, party string) (*prototk.AttestationResult, error)
	// ResolveVerifiers resolves every required verifier concurrently. It is used before delegation so
	// that assembly reads the results from PreAssembly with zero resolution work on the critical path.
	ResolveVerifiers(ctx context.Context, requiredVerifiers []*prototk.ResolveVerifierRequest) ([]*prototk.ResolvedVerifier, error)
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

func (e *engineIntegration) ResolveStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error {

	if (txn.PostAssembly.AssembleResponse.GetOutputStatesPotential() != nil && txn.PostAssembly.OutputStates == nil) ||
		(txn.PostAssembly.AssembleResponse.GetInfoStatesPotential() != nil && txn.PostAssembly.InfoStates == nil) {
		readTX := e.components.Persistence().NOTX() // no DB transaction required here for the reads from the DB (writes happen on syncpoint flusher)
		err := e.domainSmartContract.ResolvePotentialStates(ctx, e.domainStateWriter, readTX, txn)
		if err != nil {
			// Any error from ResolvePotentialStates is likely to be caused by an invalid init or assemble of the transaction
			// which is most likely a programming error in the domain or the domain manager or the sequencer
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, err)
		} else {
			log.L(ctx).Debugf("Potential states resolved for domain=%s", e.domainSmartContract.Domain().Name())
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
func (e *engineIntegration) Assemble(ctx context.Context, transactionID uuid.UUID, preAssembly *prototk.TransactionPreAssembly, resolvedVerifiers []*prototk.ResolvedVerifier, stateSnapshot *prototk.StateSnapshot, blockHeight int64) (*prototk.TransactionPostAssembly, error) {

	log.L(ctx).Debugf("Assembling transaction %s. Creating domain context with coordinator state snapshot", transactionID)

	// Create a domain context just for this call that the snapshot can be loaded into.
	dqc := e.components.StateManager().NewDomainQueryContext(ctx, e.domainSmartContract.Domain(), e.domainSmartContract.Address())
	defer dqc.Close(ctx)

	err := dqc.ImportSnapshot(ctx, stateSnapshot)
	if err != nil {
		return nil, err
	}

	// Verifiers were resolved before delegation and passed in, so assembly reads them directly with zero
	// resolution work. The state machine drops assemble requests until State_Delegated, which a transaction
	// cannot reach without first resolving its verifiers, so they are always present here.
	return e.assemble(ctx, transactionID, preAssembly, resolvedVerifiers, dqc)
}

// ResolveVerifiers resolves every required verifier concurrently via the async identity resolver and
// returns them in request order. Any single failure aborts with the first error observed.
func (e *engineIntegration) ResolveVerifiers(ctx context.Context, requiredVerifiers []*prototk.ResolveVerifierRequest) ([]*prototk.ResolvedVerifier, error) {
	if len(requiredVerifiers) == 0 {
		return nil, nil
	}
	type resolution struct {
		index    int
		verifier string
		err      error
	}
	results := make(chan resolution, len(requiredVerifiers))
	for i, v := range requiredVerifiers {
		log.L(ctx).Debugf("resolving required verifier %s", v.Lookup)
		e.components.IdentityResolver().ResolveVerifierAsync(ctx, v.Lookup, v.Algorithm, v.VerifierType,
			func(_ context.Context, verifier string) {
				results <- resolution{index: i, verifier: verifier}
			},
			func(_ context.Context, err error) {
				results <- resolution{index: i, err: err}
			},
		)
	}
	resolvedVerifiers := make([]*prototk.ResolvedVerifier, len(requiredVerifiers))
	var firstErr error
	for range requiredVerifiers {
		r := <-results
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		v := requiredVerifiers[r.index]
		resolvedVerifiers[r.index] = &prototk.ResolvedVerifier{
			Lookup:       v.Lookup,
			Algorithm:    v.Algorithm,
			VerifierType: v.VerifierType,
			Verifier:     r.verifier,
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return resolvedVerifiers, nil
}

func (e *engineIntegration) resolveLocalTransaction(ctx context.Context, transactionID uuid.UUID) (*components.ResolvedTransaction, error) {
	locallyResolvedTx, err := e.components.TxManager().GetResolvedTransactionByID(ctx, transactionID)
	if err == nil && locallyResolvedTx == nil {
		err = i18n.WrapError(ctx, err, msgs.MsgSequencerAssembleTxnNotFound, transactionID)
	}
	return locallyResolvedTx, err
}

func (e *engineIntegration) assemble(ctx context.Context, transactionID uuid.UUID, preAssembly *prototk.TransactionPreAssembly, resolvedVerifiers []*prototk.ResolvedVerifier, domainQueryContext components.DomainQueryContext) (*prototk.TransactionPostAssembly, error) {
	localTx, err := e.resolveLocalTransaction(ctx, transactionID)
	if err != nil || localTx.Transaction.Domain != e.domainSmartContract.Domain().Name() || localTx.Transaction.To == nil || *localTx.Transaction.To != e.domainSmartContract.Address() {
		if err == nil {
			log.L(ctx).Errorf("transaction %s for invalid domain/address domain=%s (expected=%s) to=%s (expected=%s)",
				transactionID, localTx.Transaction.Domain, e.domainSmartContract.Domain().Name(), localTx.Transaction.To, e.domainSmartContract.Address())
		}
		err := i18n.WrapError(ctx, err, msgs.MsgSequencerAssembleRequestInvalid, transactionID)
		return nil, err
	}

	/*
	 * Assemble
	 */
	log.L(ctx).Debugf("Assembling transaction %s", transactionID)
	assemblyResponse, err := e.domainSmartContract.AssembleTransaction(ctx, domainQueryContext, e.components.Persistence().NOTX(), transactionID, preAssembly, localTx, resolvedVerifiers)
	if err != nil {
		log.L(ctx).Errorf("error assembling transaction: %s", err)
		return nil, err
	}
	if assemblyResponse == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "AssembleTransaction returned nil")
	}

	// Some validation that we are confident we can execute the given attestation plan
	for _, attRequest := range assemblyResponse.GetAttestationPlan() {
		switch attRequest.AttestationType {
		case prototk.AttestationType_ENDORSE:
		case prototk.AttestationType_SIGN:
		default:
			errorMessage := fmt.Sprintf("Unsupported attestation type: %s", attRequest.AttestationType)
			log.L(ctx).Error(errorMessage)
			return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, errorMessage)
		}
	}

	assemblyResponse.ResolvedVerifiers = resolvedVerifiers

	log.L(ctx).Debugf("Assembled transaction %s, result: %s", transactionID, assemblyResponse.GetAssemblyResult())
	return assemblyResponse, nil
}

// SignAttestation signs a single SIGN attestation request for the given party. It returns (nil, nil) when
// the party is not local to this node, preserving the behaviour that remote SIGN parties are not signed
// locally — under the push model only the originating node produces and pushes signatures.
func (e *engineIntegration) SignAttestation(ctx context.Context, transactionID uuid.UUID, attRequest *prototk.AttestationRequest, party string) (*prototk.AttestationResult, error) {
	log.L(ctx).Debugf("validating identity locator for signing party %s", party)
	unqualifiedLookup, signerNode, err := pldtypes.PrivateIdentityLocator(party).Validate(ctx, e.nodeName, true)
	if err != nil {
		log.L(ctx).Errorf("failed to validate identity locator for signing party %s: %s", party, err)
		return nil, err
	}
	if signerNode != e.nodeName {
		log.L(ctx).Warnf("ignoring sign request of transaction %s for remote party %s ", transactionID, party)
		return nil, nil
	}
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

	return &prototk.AttestationResult{
		Name:            attRequest.Name,
		AttestationType: attRequest.AttestationType,
		Verifier: &prototk.ResolvedVerifier{
			Lookup:       party,
			Algorithm:    attRequest.Algorithm,
			Verifier:     resolvedKey.Verifier.Verifier,
			VerifierType: attRequest.VerifierType,
		},
		Payload:     signaturePayload,
		PayloadType: &attRequest.PayloadType,
	}, nil
}
