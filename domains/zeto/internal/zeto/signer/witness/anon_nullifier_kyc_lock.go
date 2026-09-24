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
