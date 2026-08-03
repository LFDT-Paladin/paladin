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

package coordinator

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	engineProto "github.com/LFDT-Paladin/paladin/core/pkg/proto/engine"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// validator_IsPrivateStateDataPendingForEndorsement returns true when private state data is
// pending arrival at this node up to the coordinator's low watermark (coordinatorBlockHeight - tolerance).
func validator_IsPrivateStateDataPendingForEndorsement(ctx context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*EndorsementRequestReceivedEvent)
	lowWatermark := e.CoordinatorBlockHeight - e.BlockHeightTolerance
	complete, err := c.engineIntegration.CheckPendingPrivateStateData(ctx, lowWatermark)
	return !complete, err
}

// action_RejectEndorsementPrivateStateDataPending sends an EndorsementRejection with reason
// PrivateStateDataPending to the requester. The endorser stays in its current state; the
// coordinator will retry once the pending private state data has arrived.
func action_RejectEndorsementPrivateStateDataPending(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*EndorsementRequestReceivedEvent)
	log.L(ctx).Warnf("rejecting endorsement request from %s due to pending private state data (coordinator=%d, endorser=%d, tolerance=%d)",
		e.FromNode, e.CoordinatorBlockHeight, c.currentBlockHeight, e.BlockHeightTolerance)
	return c.transportWriter.SendEndorsementRejection(ctx, e.FromNode, &engineProto.EndorsementRejection{
		TransactionId:          e.TransactionId,
		IdempotencyKey:         e.IdempotencyKey,
		ContractAddress:        c.contractAddress.HexString(),
		AttestationRequestName: e.AttestationRequest.Name,
		Party:                  e.Party,
		RejectionReason:        engineProto.RejectionReason_PRIVATE_STATE_DATA_PENDING,
		CoordinatorBlockHeight: e.CoordinatorBlockHeight,
		EndorserBlockHeight:    int64(c.currentBlockHeight),
		BlockHeightTolerance:   e.BlockHeightTolerance,
	})
}

// validator_IsEndorsementBlockHeightToleranceExceeded returns true when the absolute difference
// between this coordinator's stored block height (refreshed by action_RefreshBlockHeight)
// and the requesting coordinator's block height exceeds the configured block height tolerance.
func validator_IsEndorsementBlockHeightToleranceExceeded(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*EndorsementRequestReceivedEvent)
	localHeight := uint64(c.currentBlockHeight)
	remoteHeight := uint64(e.CoordinatorBlockHeight)
	diff := max(localHeight, remoteHeight) - min(localHeight, remoteHeight)
	return diff > c.blockHeightTolerance, nil
}

// action_RejectEndorsementBlockHeight sends a dedicated EndorsementRejection message indicating
// that the sender and receiver block heights differ by more than the configured tolerance. It
// does not call the domain.
func action_RejectEndorsementBlockHeight(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*EndorsementRequestReceivedEvent)
	log.L(ctx).Warnf("rejecting endorsement request from %s due to block height tolerance (coordinator=%d, endorser=%d, tolerance=%d)", e.FromNode, e.CoordinatorBlockHeight, c.currentBlockHeight, c.blockHeightTolerance)
	return c.transportWriter.SendEndorsementRejection(ctx, e.FromNode, &engineProto.EndorsementRejection{
		TransactionId:          e.TransactionId,
		IdempotencyKey:         e.IdempotencyKey,
		ContractAddress:        c.contractAddress.HexString(),
		AttestationRequestName: e.AttestationRequest.Name,
		Party:                  e.Party,
		RejectionReason:        engineProto.RejectionReason_BLOCK_HEIGHT_TOLERANCE,
		CoordinatorBlockHeight: e.CoordinatorBlockHeight,
		EndorserBlockHeight:    c.currentBlockHeight,
		BlockHeightTolerance:   int64(c.blockHeightTolerance),
	})
}

// action_RejectEndorsementEndorserIsActiveCoordinator sends an EndorsementRejection to the
// requester with reason EndorserIsActiveCoordinator: this node is currently the active
// coordinator (or becoming one) and therefore cannot act as an endorser. The sender should
// re-route the request once a new active coordinator has been established.
func action_RejectEndorsementEndorserIsActiveCoordinator(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*EndorsementRequestReceivedEvent)
	log.L(ctx).Warnf("rejecting endorsement request from %s: this node is the active coordinator", e.FromNode)
	return c.transportWriter.SendEndorsementRejection(ctx, e.FromNode, &engineProto.EndorsementRejection{
		TransactionId:          e.TransactionId,
		IdempotencyKey:         e.IdempotencyKey,
		ContractAddress:        c.contractAddress.HexString(),
		AttestationRequestName: e.AttestationRequest.Name,
		Party:                  e.Party,
		RejectionReason:        engineProto.RejectionReason_ENDORSER_IS_ACTIVE_COORDINATOR,
	})
}

// validator_IsEndorsementRequestFromHigherPriorityCoordinator returns true when the node
// that sent the endorsement request has strictly higher priority (lower index) than this
// node in the current coordinator priority list. Uses the same comparison as
// validator_IsHandoverRequestFromHigherPriorityCoordinator: we compare the sender against
// c.nodeName (not against c.currentActiveCoordinator), since the question is whether the
// requester outranks us.
func validator_IsEndorsementRequestFromHigherPriorityCoordinator(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*EndorsementRequestReceivedEvent)
	return common.IsHigherPriority(c.coordinatorPriorityList, e.FromNode, c.nodeName), nil
}

// validator_IsEndorsementRequestFromSelf returns true when this node sent the endorsement
// request itself — i.e. the coordinator and the endorser are the same node. Used in Active
// and Active_Flush so the node can endorse its own transactions without stepping down.
func validator_IsEndorsementRequestFromSelf(_ context.Context, c *coordinator, event common.Event) (bool, error) {
	e := event.(*EndorsementRequestReceivedEvent)
	return e.FromNode == c.nodeName, nil
}

// action_UpdateActiveCoordinatorFromEndorsementRequest records the sender of an endorsement
// request as the current active coordinator. Called in states where this node is not active
// (Idle, Observing, Closing_Flush, Closing) or is stepping down in response to a higher-priority
// requester (Elect, Prepared, Active, Active_Flush).
func action_UpdateActiveCoordinatorFromEndorsementRequest(_ context.Context, c *coordinator, event common.Event) error {
	e := event.(*EndorsementRequestReceivedEvent)
	c.currentActiveCoordinator = e.FromNode
	return nil
}

// action_AddEndorsementRequestSenderToEndorserCandidates adds the sender of an incoming
// endorsement request to the endorser candidate pool when it is not already known.
func action_AddEndorsementRequestSenderToEndorserCandidates(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*EndorsementRequestReceivedEvent)
	c.updateEndorserCandidates(ctx, e.FromNode)
	return nil
}

// action_HandleEndorsementRequest spawns a background goroutine to perform the
// domain-level endorsement work and send the response. This keeps the coordinator event loop
// unblocked while allowing multiple endorsements to run concurrently.
//
// The goroutine uses c.components and c.transportWriter directly. Both are safe to call from
// concurrent goroutines: remote sends go through TransportManager.Send (goroutine-safe) and
// loopback sends go through a buffered channel.
func action_HandleEndorsementRequest(ctx context.Context, c *coordinator, event common.Event) error {
	e := event.(*EndorsementRequestReceivedEvent)
	endorseCtx := ctx
	cancel := func() {}
	if !e.Expiry.IsZero() {
		endorseCtx, cancel = context.WithDeadline(ctx, e.Expiry)
	}
	go func() {
		defer cancel()
		c.handleEndorsementRequest(endorseCtx, e)
	}()
	return nil
}

func (c *coordinator) handleEndorsementRequest(ctx context.Context, e *EndorsementRequestReceivedEvent) {
	sendErr := func(errMsg string) {
		log.L(ctx).Errorf("handleEndorsementRequest error for tx %s: %s", e.TransactionId, errMsg)
		if err := c.transportWriter.SendEndorsementError(ctx, e.FromNode, &engineProto.EndorsementError{
			TransactionId:          e.TransactionId,
			IdempotencyKey:         e.IdempotencyKey,
			ContractAddress:        c.contractAddress.HexString(),
			ErrorMessage:           errMsg,
			Party:                  e.Party,
			AttestationRequestName: e.AttestationRequest.Name,
		}); err != nil {
			log.L(ctx).Errorf("handleEndorsementRequest failed to send endorsement error: %s", err)
		}
	}

	unqualifiedLookup, err := pldtypes.PrivateIdentityLocator(e.Party).Identity(ctx)
	if err != nil {
		sendErr(err.Error())
		return
	}
	resolvedSigner, err := c.components.KeyManager().ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, e.AttestationRequest.Algorithm, e.AttestationRequest.VerifierType)
	if err != nil {
		sendErr(err.Error())
		return
	}
	endorsementRequest := e.PrivateEndorsementRequest
	endorsementRequest.Endorser = &prototk.ResolvedVerifier{
		Lookup:       e.Party,
		Algorithm:    e.AttestationRequest.Algorithm,
		Verifier:     resolvedSigner.Verifier.Verifier,
		VerifierType: e.AttestationRequest.VerifierType,
	}

	dc := c.components.StateManager().NewDomainQueryContext(ctx, c.domainAPI.Domain(), c.domainAPI.Address())
	defer dc.Close(ctx)

	endorsementResult, err := c.domainAPI.EndorseTransaction(ctx, dc, c.components.Persistence().NOTX(), endorsementRequest)
	if err != nil {
		sendErr(err.Error())
		return
	}
	e.AttestationRequest.Payload = endorsementResult.Payload

	attResult := &prototk.AttestationResult{
		Name:            e.AttestationRequest.Name,
		AttestationType: e.AttestationRequest.AttestationType,
		Verifier:        endorsementResult.Endorser,
	}

	revertReason := ""

	switch endorsementResult.Result {
	case prototk.EndorseTransactionResponse_REVERT:
		revertReason = "(no revert reason)"
		if endorsementResult.RevertReason != nil {
			revertReason = *endorsementResult.RevertReason
		}
	case prototk.EndorseTransactionResponse_SIGN:
		unqualifiedLookup, signerNode, err := pldtypes.PrivateIdentityLocator(endorsementResult.Endorser.Lookup).Validate(ctx, c.nodeName, true)
		if err != nil {
			sendErr(err.Error())
			return
		}
		if signerNode == c.nodeName {
			log.L(ctx).Info("endorsement response signing request includes us - signing it now")
			keyMgr := c.components.KeyManager()
			resolvedKey, err := keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, e.AttestationRequest.Algorithm, e.AttestationRequest.VerifierType)
			if err != nil {
				sendErr(err.Error())
				return
			}
			signaturePayload, err := keyMgr.Sign(ctx, resolvedKey, e.AttestationRequest.PayloadType, e.AttestationRequest.Payload)
			if err != nil {
				sendErr(err.Error())
				return
			}
			attResult.Payload = signaturePayload
		} else {
			// This can presumably never happen, since this endorsement request came to us
			log.L(ctx).Errorf("handleEndorsementRequest received isn't for this node: %s", signerNode)
		}
	case prototk.EndorseTransactionResponse_ENDORSER_SUBMIT:
		attResult.Constraints = append(attResult.Constraints, prototk.AttestationResult_ENDORSER_MUST_SUBMIT)
	}

	c.metrics.IncEndorsedTransactions()
	msg := &engineProto.EndorsementResponse{
		Endorsement:            attResult,
		TransactionId:          e.TransactionId,
		IdempotencyKey:         e.IdempotencyKey,
		AttestationRequestName: e.AttestationRequest.Name,
		Party:                  e.Party,
		ContractAddress:        c.contractAddress.HexString(),
	}
	if revertReason != "" {
		msg.RevertReason = &revertReason
	}
	if err = c.transportWriter.SendEndorsementResponse(ctx, e.FromNode, msg); err != nil {
		log.L(ctx).Errorf("handleEndorsementRequest failed to send endorsement response: %s", err)
	}
}
