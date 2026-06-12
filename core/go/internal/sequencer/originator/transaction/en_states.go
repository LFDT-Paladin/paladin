// Copyright © 2026 Kaleido, Inc.
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

package transaction

import (
	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"golang.org/x/text/language"
)

var pdm = func(key, translation string) i18n.MessageKey {
	return i18n.PDM(language.AmericanEnglish, key, translation)
}

var (
	MsgStateInitial              = pdm("originator.transaction.State_Initial", "Transaction state machine created")
	MsgStatePending              = pdm("originator.transaction.State_Pending", "The transaction has not yet been delegated to a coordinator")
	MsgStateDelegated            = pdm("originator.transaction.State_Delegated", "The transaction has been sent to the current active coordinator")
	MsgStateAssembling           = pdm("originator.transaction.State_Assembling", "The coordinator has sent an assemble request to us and we have not yet sent the assembled transaction back to the coordinator")
	MsgStateEndorsementGathering = pdm("originator.transaction.State_Endorsement_Gathering", "An assemble response has been sent to the active coordinator, who should now be gathering endorsements for the transaction. A dispatch confirmation request is expected in this state.")
	MsgStatePrepared             = pdm("originator.transaction.State_Prepared", "We know that the coordinator has got as far as preparing a public transaction for this transaction")
	MsgStateDispatched           = pdm("originator.transaction.State_Dispatched", "The active coordinator that this transaction was delegated to has dispatched the transaction to a public transaction manager for submission to the base ledger")
	MsgStateSequenced            = pdm("originator.transaction.State_Sequenced", "The public transaction manager at the coordinator has allocated a nonce for this transaction's base ledger transaction")
	MsgStateSubmitted            = pdm("originator.transaction.State_Submitted", "The base ledger transaction has been submitted to the blockchain")
	MsgStateConfirmed            = pdm("originator.transaction.State_Confirmed", "The base ledger transaction has been confirmed by the blockchain as successful")
	MsgStateReverted             = pdm("originator.transaction.State_Reverted", "Upon attempting to assemble the transaction, the domain code has determined that the intent is not valid and the transaction is finalized as reverted")
	MsgStateParked               = pdm("originator.transaction.State_Parked", "Upon attempting to assemble the transaction, the domain code has determined that the transaction is not ready to be assembled and it is parked for later processing. Other transactions for the current originator can continue unless they have an explicit dependency on this transaction.")
	MsgStateFinal                = pdm("originator.transaction.State_Final", "Final state for the transaction. Transactions are removed from memory as soon as they enter this state")
)
