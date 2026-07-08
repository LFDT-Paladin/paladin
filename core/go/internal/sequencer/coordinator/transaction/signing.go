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
package transaction

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// applySignature appends a pushed signature to the assembled plan's Signatures — the same slice the
// endorsement requests read ([endorsing.go] Signatures) — ignoring duplicates so a re-pushed signature is a
// no-op. A nil signature is ignored defensively.
func (t *coordinatorTransaction) applySignature(ctx context.Context, signature *prototk.AttestationResult) {
	if signature == nil {
		return
	}
	for _, existing := range t.pt.PostAssembly.AssembleResponse.GetSignatures() {
		if existing.Name == signature.Name && verifierLookup(existing) == verifierLookup(signature) {
			log.L(ctx).Debugf("ignoring duplicate signature %s from %s for transaction %s", signature.Name, verifierLookup(signature), t.pt.ID)
			return
		}
	}
	log.L(ctx).Debugf("applying signature %s from %s for transaction %s", signature.Name, verifierLookup(signature), t.pt.ID)
	t.pt.PostAssembly.AssembleResponse.Signatures = append(t.pt.PostAssembly.AssembleResponse.Signatures, signature)
}

func verifierLookup(r *prototk.AttestationResult) string {
	if r.Verifier == nil {
		return ""
	}
	return r.Verifier.Lookup
}

func (t *coordinatorTransaction) signRequirementsFulfilled(ctx context.Context) bool {
	return len(t.unfulfilledSignRequirements(ctx)) == 0
}

// unfulfilledSignRequirements returns the SIGN attestations that still need a signature. It counts only
// SIGN attestations whose party is on the originating node, symmetric with the originator's
// guard_HasLocalSignRequirement and with engine integration's SignAttestation locality check. A SIGN party
// on any node other than the originator is skipped (never counted as outstanding): under the push model only
// the originator produces and pushes signatures, so a domain that requests a signature from a non-originator
// node is fundamentally unsupported — counting it would strand the transaction in State_Signing until it
// times out and repools. A SIGN attestation is fulfilled once at least one collected signature matches its
// Name (each shipped SIGN attestation is signed once by the sender/originator.
func (t *coordinatorTransaction) unfulfilledSignRequirements(ctx context.Context) []*prototk.AttestationRequest {
	unfulfilled := make([]*prototk.AttestationRequest, 0)
	if t.pt.PostAssembly == nil {
		return unfulfilled
	}
	for _, attRequest := range t.pt.PostAssembly.AssembleResponse.GetAttestationPlan() {
		if attRequest.AttestationType != prototk.AttestationType_SIGN {
			continue
		}
		hasOriginatorParty := false
		for _, party := range attRequest.Parties {
			_, signerNode, err := pldtypes.PrivateIdentityLocator(party).Validate(ctx, t.originatorNode, true)
			if err != nil {
				log.L(ctx).Warnf("could not validate identity locator for signing party %q: %v", party, err)
				continue
			}
			if signerNode == t.originatorNode {
				hasOriginatorParty = true
			} else {
				// Fundamentally unsupported: only the originator pushes signatures, so a SIGN party on a
				// different node can never be fulfilled. Skip it rather than strand the transaction.
				log.L(ctx).Warnf("ignoring SIGN requirement %s for party %s on non-originator node %s (originator node=%s): unsupported", attRequest.Name, party, signerNode, t.originatorNode)
			}
		}
		if !hasOriginatorParty {
			continue
		}
		fulfilled := false
		for _, signature := range t.pt.PostAssembly.AssembleResponse.GetSignatures() {
			if signature.Name == attRequest.Name {
				fulfilled = true
				break
			}
		}
		if !fulfilled {
			unfulfilled = append(unfulfilled, attRequest)
		}
	}
	return unfulfilled
}

func guard_SignRequirementsFulfilled(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.signRequirementsFulfilled(ctx)
}

func action_Signed(ctx context.Context, t *coordinatorTransaction, event common.Event) error {
	e := event.(*SignedEvent)
	t.applySignature(ctx, e.AttestationResult)
	return nil
}

// action_SignError increments the shared assemble error count so a pushed signing failure is bounded by the
// same retry budget as an assemble failure (guard_CanRetryErroredAssemble).
func action_SignError(_ context.Context, t *coordinatorTransaction, _ common.Event) error {
	t.assembleErrorCount++
	return nil
}
