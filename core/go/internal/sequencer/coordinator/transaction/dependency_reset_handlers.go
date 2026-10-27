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
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/statemachine"
)

// The dependency reset/revert escapes shared by every post-assembly state (Signing, Endorsement_Gathering,
// Blocked, Confirming_Dispatchable, Ready_For_Dispatch). Once a transaction is past assembly:
// - a chained dependency becoming unassembled sends it back to PreAssembly_Blocked (its chained dependency
//   must be re-assembled first), and
// - a post-assembly dependency reset sends it to Pooled (post-assembly dependencies are cleared and it is
//   too far along to have preassembly dependencies).
// A chained-dependency failure reverts the transaction.

// dependencyResetHandler handles Event_DependencyReset for a post-assembly state.
var dependencyResetHandler = dependencyResetToPreAssemblyOrPool()

// dependencyRevertedHandler handles Event_DependencyConfirmedReverted for a post-assembly state.
var dependencyRevertedHandler = dependencyResetToPreAssemblyOrPool()

// chainedDependencyFailedHandler handles Event_ChainedDependencyFailed for a post-assembly state.
var chainedDependencyFailedHandler = EventHandlers{
	Match: statemachine.MatchFirst,
	Handlers: []EventHandler{{
		Actions: []ActionRule{
			{Action: action_FinalizeOnChainedDependencyFailure},
			{Action: action_NotifyOriginatorOfChainedDependencyFailure},
		},
		Transitions: []Transition{{To: State_Reverted}},
	}},
}

func dependencyResetToPreAssemblyOrPool() EventHandlers {
	return EventHandlers{
		Match: statemachine.MatchFirst,
		Handlers: []EventHandler{{
			Validator: validator_IsChainedDependency,
			Actions: []ActionRule{{
				Action: action_MarkChainedDependencyUnassembled,
			}},
			Transitions: []Transition{{
				To:      State_PreAssembly_Blocked,
				Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
			}},
		}, {
			Validator: statemachine.ValidatorNot(validator_IsChainedDependency),
			Transitions: []Transition{{
				To:      State_Pooled,
				Actions: []ActionRule{{Action: action_NotifyDependentsOfReset}},
			}},
		}},
	}
}
