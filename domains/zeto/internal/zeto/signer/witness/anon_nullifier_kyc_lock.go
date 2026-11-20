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

package witness

import (
	"context"

	pb "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/proto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
)

// FungibleLockKycWitnessInputs assembles the anon_nullifier_kyc_transferLocked circuit witness.
// Locked-input spend uses commitment-based inputs and KYC inclusion only (no nullifier or UTXO SMT signals).
type FungibleLockKycWitnessInputs struct {
	FungibleWitnessInputs
	Extras *pb.ProvingRequestExtras_NullifiersKyc
}

func (inputs *FungibleLockKycWitnessInputs) Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	m, err := inputs.FungibleWitnessInputs.Assemble(ctx, keyEntry)
	if err != nil {
		return nil, err
	}
	var kycHelper FungibleNullifierKycWitnessInputs
	kycRoot, kycProofs, _, err := kycHelper.decodeSmtProofObject(ctx, inputs.Extras.SmtKycProof)
	if err != nil {
		return nil, err
	}
	m["identitiesRoot"] = kycRoot
	m["identitiesMerkleProof"] = kycProofs
	return m, nil
}
