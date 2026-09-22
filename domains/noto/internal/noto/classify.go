// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package noto

import (
	"errors"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// Assemble and endorse each have two possible outcomes for a failure. A revert says the
// transaction is invalid and must never be submitted, so the platform fails it immediately
// and reports the reason to the application. A plain error says something unexpected went
// wrong here and now, so the platform retries. Choosing wrongly is expensive in both
// directions: an invalid transaction classified as an error retries until it times out,
// and an unexpected failure classified as a revert terminally fails a transaction that
// would have succeeded on the next attempt.
//
// Which outcome a failure results in depends on who supplied the data the check rejected,
// and that differs by phase. At assemble the originator's own node built the transaction
// specification, resolved the verifiers, selected the states and read the contract config,
// so only the function parameters and the operation they request came from outside. At
// endorse everything in the request is the originator's proposal, and only the endorser's
// own state store reads and callbacks are trusted. Each Noto message that reverts has
// a row saying in which phases it does so. Anything assemble can reject is still invalid
// at endorse, so no row reverts at assemble without also reverting at endorse.
//
// The table records reverts only. Every error not carrying a listed Noto message is
// internal, so the platform retries it, and nothing checks that this was a decision rather
// than an omission. That includes Noto messages that are not listed, and errors from other
// components that reach these paths unwrapped - pldtypes, the toolkit, the signer, the JSON
// and ABI encoders, and core failing a callback, which the toolkit surfaces as an unkeyed
// string - none of which can ever have a row. Where such a failure is the transaction's
// fault, the site must wrap it in a Noto message that is listed. Paladin's i18n errors do
// not unwrap, so only the outermost message is visible: wrapping a keyed cause in another
// Noto message reclassifies it as that message.

type revertsAt struct{ assemble, endorse bool }

var reverts = map[i18n.ErrorMessageKey]revertsAt{
	// Function selection and parameters, supplied by the application
	msgs.MsgUnknownFunction:             {assemble: true, endorse: true},
	msgs.MsgUnexpectedFunctionSignature: {assemble: true, endorse: true},
	msgs.MsgInvalidParams:               {assemble: true, endorse: true},
	msgs.MsgParameterRequired:           {assemble: true, endorse: true},
	msgs.MsgParameterGreaterThanZero:    {assemble: true, endorse: true},
	msgs.MsgInvalidDelegate:             {assemble: true, endorse: true},
	msgs.MsgUnknownDomainVariant:        {assemble: true, endorse: true},

	// The requested operation is not permitted for the sender
	msgs.MsgMintOnlyNotary:    {assemble: true, endorse: true},
	msgs.MsgBurnNotAllowed:    {assemble: true, endorse: true},
	msgs.MsgLockNotAllowed:    {assemble: true, endorse: true},
	msgs.MsgUnlockOnlyCreator: {assemble: true, endorse: true},

	// The states in the transaction do not satisfy the operation. MsgInvalidStateData and
	// MsgInvalidLockState are reverts because the state came from the transaction; the same
	// parse failure on a state read from this node's own store carries a stored-state
	// message instead, which is internal.
	msgs.MsgInvalidInputs:              {assemble: true, endorse: true},
	msgs.MsgInvalidAmount:              {assemble: true, endorse: true},
	msgs.MsgInsufficientFunds:          {assemble: true, endorse: true},
	msgs.MsgDuplicateStateInList:       {assemble: true, endorse: true},
	msgs.MsgInvalidListInput:           {assemble: true, endorse: true},
	msgs.MsgUnexpectedSchema:           {assemble: true, endorse: true},
	msgs.MsgNoStatesSpecified:          {assemble: true, endorse: true},
	msgs.MsgDuplicateNullifierInList:   {assemble: true, endorse: true},
	msgs.MsgIncompleteCoinForNullifier: {assemble: true, endorse: true},
	msgs.MsgInvalidStateData:           {assemble: true, endorse: true},
	msgs.MsgStateWrongOwner:            {assemble: true, endorse: true},
	msgs.MsgLockIDNotFound:             {assemble: true, endorse: true},
	msgs.MsgInvalidLockTransition:      {assemble: true, endorse: true},
	msgs.MsgInvalidLockStateLockID:     {assemble: true, endorse: true},
	msgs.MsgInvalidLockState:           {assemble: true, endorse: true},

	// The originator's signature over the transaction, which only endorse checks
	msgs.MsgAttestationNotFound:   {assemble: false, endorse: true},
	msgs.MsgInvalidSignature:      {assemble: false, endorse: true},
	msgs.MsgSignatureDoesNotMatch: {assemble: false, endorse: true},

	// Values this node produced or read from its own store at assemble, but which the
	// originator supplies at endorse
	msgs.MsgErrorVerifyingAddress:  {assemble: false, endorse: true},
	msgs.MsgInvalidTransactionSpec: {assemble: false, endorse: true},
	msgs.MsgInvalidTransactionData: {assemble: false, endorse: true},
}

func classify(err error) revertsAt {
	var pdErr i18n.PDError
	if !errors.As(err, &pdErr) {
		return revertsAt{}
	}
	return reverts[pdErr.MessageKey()]
}

func isAssembleRevert(err error) bool { return classify(err).assemble }
func isEndorseRevert(err error) bool  { return classify(err).endorse }

// assembleRevertOrError turns an assemble failure into either a terminal REVERT response
// carrying the reason, or an error for the platform to retry.
func assembleRevertOrError(err error) (*prototk.AssembleTransactionResponse, error) {
	if err == nil {
		return nil, nil
	}
	if isAssembleRevert(err) {
		reason := err.Error()
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   &reason,
		}, nil
	}
	return nil, err
}

// endorseRevertOrError turns an endorse failure into either a terminal REVERT response
// carrying the reason, or an error for the platform to retry.
func endorseRevertOrError(err error) (*prototk.EndorseTransactionResponse, error) {
	if err == nil {
		return nil, nil
	}
	if isEndorseRevert(err) {
		reason := err.Error()
		return &prototk.EndorseTransactionResponse{
			EndorsementResult: prototk.EndorseTransactionResponse_REVERT,
			RevertReason:      &reason,
		}, nil
	}
	return nil, err
}
