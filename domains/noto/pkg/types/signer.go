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

package types

import (
	"fmt"
	"strings"

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// NullifierSpec payload type prefix: NotoCoin state JSON signing logic
// is used to derive a spend nullifier for the target state.
//
// Only unlocked coins are nullified - locked states are spent by ID
// throughout their lifecycle - so there is no locked equivalent of this
const PAYLOAD_DOMAIN_NOTO_NULLIFIER = "domain:noto:nullifier"

// NullifierSpec verifier type: placeholder (nullifier derivation requires no external key)
const VERIFIER_DOMAIN_NOTO_NULLIFIER = "domain:noto:nullifier:verifier"

func AlgoDomainNullifier(name string) string {
	return fmt.Sprintf("domain:%s:nullifier", name)
}

// NullifierPayloadType returns the payload type for deriving the nullifier of a coin held in
// a specific contract.
//
// The contract address travels in the payload type because a nullifier must be bound to its
// contract (see calculateNullifier), but the Sign request that derives a nullifier locally
// carries only the state data - not the contract the state belongs to. The payload type is
// the one part of the NullifierSpec that is passed through to Sign unchanged, so it is where
// the binding has to ride.
func NullifierPayloadType(contractAddress *pldtypes.EthAddress) string {
	return fmt.Sprintf("%s:%s", PAYLOAD_DOMAIN_NOTO_NULLIFIER, contractAddress.String())
}

// ParseNullifierPayloadType recovers the contract address from a payload type built by
// NullifierPayloadType. A payload type without an address is rejected: deriving a nullifier
// that is not bound to a contract is exactly the flaw this encoding exists to prevent.
func ParseNullifierPayloadType(payloadType string) (*pldtypes.EthAddress, error) {
	suffix, found := strings.CutPrefix(payloadType, PAYLOAD_DOMAIN_NOTO_NULLIFIER+":")
	if !found {
		return nil, fmt.Errorf("payload type '%s' does not identify a contract", payloadType)
	}
	return pldtypes.ParseEthAddress(suffix)
}

// IsNullifierPayloadType reports whether the payload type asks for a Noto nullifier, without
// validating the contract address it carries
func IsNullifierPayloadType(payloadType string) bool {
	return strings.HasPrefix(payloadType, PAYLOAD_DOMAIN_NOTO_NULLIFIER)
}
