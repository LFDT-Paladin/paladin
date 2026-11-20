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

// Assemble and endorse each have two possible outcomes for a failure. A revert says the
// transaction is invalid and must never be submitted, so the platform fails it immediately
// and reports the reason to the application. An internal error says something unexpected went
// wrong here and now, so the platform retries. Choosing wrongly is expensive in both directions:
// an invalid transaction classified as an internal error retries until it times out, and an
// unexpected failure classified as a revert terminally fails a transaction that would have
// succeeded on the next attempt.
//
// The answer depends on who supplied the data a check rejects, and that differs by stage.
// Assemble runs on the originator, over a specification and verifier set its own node built
// around the caller's request, so only the function the caller chose and the parameters they
// supplied are untrusted. Endorse runs on an endorser, over a specification and verifier set the
// originator proposed, so everything in the request is untrusted. The same check on the same
// field can therefore be an internal bug at assemble but an invalid proposal at endorse, which is
// why each stage has its own predicate.
//
// The classification is made where the failure is detected, by the code that knows whose data
// it rejected. A function's return type states which stages reach it: AssembleError or
// EndorseError for one stage, AssembleOrEndorseError for both. The compiler then requires every
// category the function returns to answer for each stage its return type names. The reverse is
// not checked: a category answering for both stages compiles on a single-stage path, where its
// extra answer is inert. Init, prepare, call and receipt paths have no revert outcome; they
// return a plain error and discard any category they receive.
//
// Each implementation of these interfaces is a category of failure. A category implements only
// the predicates of the stages whose paths reach it. Comments on failure categories intentionally
// repeat the result and reasoning of their predicates so that they can be viewed inline in an IDE.

// AssembleError is returned on paths where assemble is the only stage that can revert.
type AssembleError interface {
	error

	// IsAssembleRevert reports whether a failing assemble should return a revert rather
	// than an error.
	IsAssembleRevert() bool
}

// EndorseError is returned on paths where endorse is the only stage that can revert.
type EndorseError interface {
	error

	// IsEndorseRevert reports whether a failing endorse should return a revert rather
	// than an error.
	IsEndorseRevert() bool
}

// AssembleOrEndorseError is returned on paths both stages can reach, so it answers for each.
type AssembleOrEndorseError interface {
	AssembleError
	EndorseError
}

// callbackFailedError: a callback to Paladin core failed.
//
// IsAssembleRevert false: the failure is in this node, not the transaction, so a retry may succeed.
//
// IsEndorseRevert false: the failure is in this node, not the proposal, so a retry may succeed.
type callbackFailedError struct{ error }

func (callbackFailedError) IsAssembleRevert() bool { return false }
func (callbackFailedError) IsEndorseRevert() bool  { return false }

// encodeFailedError: values this node has already parsed or built will not marshal or ABI-encode.
//
// IsAssembleRevert false: the values are typed before they reach the encoder, so a failure is a
// code defect rather than anything about the transaction.
//
// IsEndorseRevert false: the values are this node's parsed copy of the proposal, typed before
// they reach the encoder, so a failure is a code defect rather than anything about the proposal.
type encodeFailedError struct{ error }

func (encodeFailedError) IsAssembleRevert() bool { return false }
func (encodeFailedError) IsEndorseRevert() bool  { return false }

// invalidStoredStateError: a state read from this node's own store will not parse.
//
// IsAssembleRevert false: this node wrote the state, so the transaction did not cause the failure.
type invalidStoredStateError struct{ error }

func (invalidStoredStateError) IsAssembleRevert() bool { return false }

// invalidAssembledStateError: a state this node has just assembled has an ID that will not parse or a
// nullifier spec that does not name this contract.
//
// IsAssembleRevert false: this node built the state, so a failure is a code defect rather than
// anything about the transaction.
type invalidAssembledStateError struct{ error }

func (invalidAssembledStateError) IsAssembleRevert() bool { return false }

// invalidParamsError: the function parameters do not parse or fail the handler's own rules, or the
// contract's variant does not offer the requested function.
//
// IsAssembleRevert true: the function and its parameters are the caller's request.
//
// IsEndorseRevert true: the function and its parameters are part of the originator's proposal.
type invalidParamsError struct{ error }

func (invalidParamsError) IsAssembleRevert() bool { return true }
func (invalidParamsError) IsEndorseRevert() bool  { return true }

// unknownFunctionError: no Noto function matches the requested name and signature.
//
// IsAssembleRevert true: the caller chose the function name and signature.
//
// IsEndorseRevert true: the function name and signature are part of the originator's proposal.
type unknownFunctionError struct{ error }

func (unknownFunctionError) IsAssembleRevert() bool { return true }
func (unknownFunctionError) IsEndorseRevert() bool  { return true }

// invalidAmountsError: the inputs and outputs of the requested operation are not the set it requires,
// or do not balance with its amount.
//
// IsAssembleRevert true: the amount being checked is the caller's parameter, compared against the
// coins that exist for it.
//
// IsEndorseRevert true: the states whose amounts are checked are part of the originator's
// proposal.
type invalidAmountsError struct{ error }

func (invalidAmountsError) IsAssembleRevert() bool { return true }
func (invalidAmountsError) IsEndorseRevert() bool  { return true }

// invalidVerifierError: a resolved verifier is missing from the list or is not an eth address.
//
// IsAssembleRevert false: this node resolved every verifier its own init declared, all-or-nothing,
// before assembling, so a missing entry means init and assemble disagree about what the
// transaction needs, and a malformed one is a fault in this node's resolver.
//
// IsEndorseRevert true: the verifier list is part of the originator's proposal.
type invalidVerifierError struct{ error }

func (invalidVerifierError) IsAssembleRevert() bool { return false }
func (invalidVerifierError) IsEndorseRevert() bool  { return true }

// invalidTransactionSpecError: the function ABI, contract config or contract address in the
// transaction specification will not parse.
//
// IsAssembleRevert false: these are a round-trip of values this node marshalled itself.
//
// IsEndorseRevert true: the transaction specification is part of the originator's proposal.
type invalidTransactionSpecError struct{ error }

func (invalidTransactionSpecError) IsAssembleRevert() bool { return false }
func (invalidTransactionSpecError) IsEndorseRevert() bool  { return true }

// invalidTransactionDataError: a state ID or transaction ID will not encode as bytes32 for the
// on-chain transaction data or the lock ID.
//
// IsAssembleRevert false: this node generated the IDs.
//
// IsEndorseRevert true: the IDs are part of the originator's proposal.
type invalidTransactionDataError struct{ error }

func (invalidTransactionDataError) IsAssembleRevert() bool { return false }
func (invalidTransactionDataError) IsEndorseRevert() bool  { return true }

// operationNotAllowedError: the contract disables the requested operation, or the sender may not
// perform it.
//
// IsEndorseRevert true: the operation and the sender are part of the originator's proposal.
type operationNotAllowedError struct{ error }

func (operationNotAllowedError) IsEndorseRevert() bool { return true }

// invalidStateListError: a state list in the transaction is empty, or has a duplicate, an
// unparseable entry or the wrong schema.
//
// IsEndorseRevert true: the state lists are part of the originator's proposal.
type invalidStateListError struct{ error }

func (invalidStateListError) IsEndorseRevert() bool { return true }

// invalidSignatureError: the sender's signature is missing or does not recover to the sender.
//
// IsEndorseRevert true: the attestation is part of the originator's proposal.
type invalidSignatureError struct{ error }

func (invalidSignatureError) IsEndorseRevert() bool { return true }

// wrongOwnerError: an input being spent, or a locked or cancel output an unlock produces, is not owned
// by the party the transaction names.
//
// IsEndorseRevert true: the states and the party they are checked against are part of the
// originator's proposal.
type wrongOwnerError struct{ error }

func (wrongOwnerError) IsEndorseRevert() bool { return true }

// invalidLockTransitionError: the lock states in the transaction do not describe a permitted
// transition.
//
// IsEndorseRevert true: the lock states are part of the originator's proposal.
type invalidLockTransitionError struct{ error }

func (invalidLockTransitionError) IsEndorseRevert() bool { return true }

// lockNotFoundError: no available lock state has the requested lock ID.
//
// IsAssembleRevert true: the lock ID is the caller's request.
type lockNotFoundError struct{ error }

func (lockNotFoundError) IsAssembleRevert() bool { return true }

// insufficientFundsError: the owner named in the request has no available coins, under the requested
// lock where there is one, covering what the operation needs.
//
// IsAssembleRevert true: the owner, lock ID and amount are the caller's request.
type insufficientFundsError struct{ error }

func (insufficientFundsError) IsAssembleRevert() bool { return true }
