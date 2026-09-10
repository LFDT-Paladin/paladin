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

// NotoDomainError carries the classification of a failure alongside the failure itself.
//
// Assemble and endorse each have two possible outcomes for a failure. A revert says the
// transaction is invalid and must never be submitted, so the platform fails it immediately
// and reports the reason to the application. A plain error says something unexpected went
// wrong here and now, so the platform retries. Choosing wrongly is expensive in both
// directions: an invalid transaction classified as an error retries until it times out,
// and an unexpected failure classified as a revert terminally fails a transaction that
// would have succeeded on the next attempt.
//
// The classification therefore has to be made where the failure is detected, by whoever
// knows whose fault it is, rather than guessed at the entrypoint. Assemble and endorse
// return NotoDomainError so that the compiler requires an answer: an unclassified error will
// not satisfy the return type. Use internalErr, validationErr or endorseOnlyValidationErr
// to construct one. Each returns nil for a nil error, so they can be applied to an error that
// may or may not be set without producing a non-nil NotoDomainError holding a nil error.
//
// There are two predicates rather than one because the phase decides who supplied the data
// that a check rejects. Assemble runs on the originator, over a specification and verifier
// set the originator's own node built. Endorse runs on an endorser, over a specification
// and verifier set that the originator has proposed. So a check that fails on the same field
// can be caused by an internal bug at assembly but be a proposed invalid transaction at
// endorsement.
type NotoDomainError interface {
	error

	// IsAssembleRevert reports whether a failing assemble should return a revert rather
	// than an error. Consumed only by assembleRevertOrError.
	IsAssembleRevert() bool

	// IsEndorseRevert reports whether a failing endorse should return a revert rather
	// than an error. Consumed only by endorseRevertOrError.
	IsEndorseRevert() bool
}

type notoDomainError struct {
	err            error
	assembleRevert bool
	endorseRevert  bool
}

func (e *notoDomainError) Error() string          { return e.err.Error() }
func (e *notoDomainError) Unwrap() error          { return e.err }
func (e *notoDomainError) IsAssembleRevert() bool { return e.assembleRevert }
func (e *notoDomainError) IsEndorseRevert() bool  { return e.endorseRevert }

// internalErr classifies a failure that is nobody's fault but ours: a callback that did not
// answer, data this node itself produced that will not parse back, an invariant that a code
// change could break but a transaction could not. It reverts on neither path, so the
// platform retries. Do not reach for it as the cautious default - retrying a transaction
// that can never succeed wastes the retry budget and delays the error the application needs
// to see.
func internalErr(err error) NotoDomainError {
	if err == nil {
		return nil
	}
	return &notoDomainError{err: err, assembleRevert: false, endorseRevert: false}
}

// validationErr classifies a failure that the transaction is responsible for on every path:
// parameters that do not meet the contract's rules- e.g. amounts that do not balance, a signature
// that does not verify, states whose owner is not the party spending them. It reverts on
// both paths.
func validationErr(err error) NotoDomainError {
	if err == nil {
		return nil
	}
	return &notoDomainError{err: err, assembleRevert: true, endorseRevert: true}
}

// endorseOnlyValidationErr classifies a failure where the culprit depends on the phase: the
// data being rejected reaches an endorser from the coordinator, so at endorse it is the
// originating application's, but on the originator's own node the same data came from local
// state and could only be wrong through a bug in Noto. It reverts at endorse and errors at
// assemble.
//
// The mirror case - a revert at assemble but an error at endorse - does not arise. Anything
// assemble can prove invalid was supplied by the application, and at endorse that same data
// has crossed the wire without becoming any more trustworthy.
func endorseOnlyValidationErr(err error) NotoDomainError {
	if err == nil {
		return nil
	}
	return &notoDomainError{err: err, assembleRevert: false, endorseRevert: true}
}
