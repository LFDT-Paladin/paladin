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
// it rejected, as a literal of the error type for the stages that reach it with the answer for
// each of those stages written out. A function's return type states which stages reach it:
// AssembleError or EndorseError for one stage, AssembleOrEndorseError for both. The compiler
// then requires every value the function returns to answer for each stage its return type names.
// The reverse is not checked: a value answering for both stages compiles on a single-stage path,
// where its extra answer is inert, so a single-stage path builds the error type of its own
// stage. Init, prepare, call and receipt paths have no revert outcome; they return a plain error
// and discard any classification they receive.
//
// A failure is built only inside the branch that has detected it, so the error it wraps is never
// nil and a nil interface value is the only representation of success.

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

type assembleError struct {
	error
	revert bool
}

func (f assembleError) IsAssembleRevert() bool { return f.revert }

type endorseError struct {
	error
	revert bool
}

func (f endorseError) IsEndorseRevert() bool { return f.revert }

type assembleOrEndorseError struct {
	error
	assembleRevert bool
	endorseRevert  bool
}

func (f assembleOrEndorseError) IsAssembleRevert() bool { return f.assembleRevert }
func (f assembleOrEndorseError) IsEndorseRevert() bool  { return f.endorseRevert }
