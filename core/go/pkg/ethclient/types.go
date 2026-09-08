/*
 * Copyright © 2024 Kaleido, Inc.
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

package ethclient

import (
	"strings"
)

// ErrorReason are a set of standard error conditions that a blockchain connector can return
// from execution, that affect the action of the transaction manager to the response.
// It is important that error mapping is performed for each of these classification
type ErrorReason string

// *** MUST UPDATE MapSubmissionRejected IF ADDING NEW REASONS THAT ARE POSSIBLE DURING TRANSACTION PREPARE PHASE OF SUBMISSION ***
const (
	// ErrorReasonInvalidInputs transaction inputs could not be parsed by the connector according to the interface (nothing was sent to the blockchain)
	ErrorReasonInvalidInputs ErrorReason = "invalid_inputs"
	// ErrorReasonTransactionReverted on-chain execution (only expected to be returned when the connector is doing gas estimation, or executing a query)
	ErrorReasonTransactionReverted ErrorReason = "transaction_reverted"
	// ErrorReasonNonceTooLow on transaction submission, if the nonce has already been used for a transaction that has made it into a block on the canonical chain known to the local node
	ErrorReasonNonceTooLow ErrorReason = "nonce_too_low"
	// ErrorReasonTransactionUnderpriced if the transaction is rejected due to too low gas price. Either because it was too low according to the minimum configured on the node, or because it's a rescue transaction without a price bump.
	ErrorReasonTransactionUnderpriced ErrorReason = "transaction_underpriced"
	// ErrorReasonInsufficientFunds if the transaction is rejected due to not having enough of the underlying network coin (ether etc.) in your wallet
	ErrorReasonInsufficientFunds ErrorReason = "insufficient_funds"
	// ErrorReasonNotFound if the requested object (block/receipt etc.) was not found
	ErrorReasonNotFound ErrorReason = "not_found"
	// ErrorKnownTransaction if the exact transaction is already known
	ErrorKnownTransaction ErrorReason = "known_transaction"
	// ErrorReasonDownstreamDown if the downstream JSONRPC endpoint is down
	ErrorReasonDownstreamDown ErrorReason = "downstream_down"
)

func MapSubmissionRejected(err error) bool {
	reason := MapError(err)
	switch reason {
	case ErrorReasonInvalidInputs,
		ErrorReasonTransactionReverted,
		ErrorReasonInsufficientFunds:
		// These reason codes are considered as rejections of the transaction - see SubmissionError
		return true
	default:
		// Everything else is eligible for idempotent retry of submission
		return false
	}
}

func MapError(err error) ErrorReason {

	errString := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errString, "filter not found"):
		return ErrorReasonNotFound
	case strings.Contains(errString, "nonce too low"):
		return ErrorReasonNonceTooLow
	case strings.Contains(errString, "insufficient funds"):
		return ErrorReasonInsufficientFunds
	case strings.Contains(errString, "transaction underpriced"):
		return ErrorReasonTransactionUnderpriced
	case strings.Contains(errString, "known transaction"):
		return ErrorKnownTransaction
	case strings.Contains(errString, "already known"):
		return ErrorKnownTransaction
	case strings.Contains(errString, "reverted"):
		return ErrorReasonTransactionReverted
	// https://docs.avax.network/quickstart/integrate-exchange-with-avalanche#determining-finality
	case strings.Contains(errString, "cannot query unfinalized data"):
		return ErrorReasonNotFound
	case strings.Contains(errString, "the method net_version does not exist/is not available"):
		return ErrorReasonNotFound
	default:
		// default to no mapping
		return ""
	}
}
