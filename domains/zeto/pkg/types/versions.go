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

package types

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
)

// A Zeto pool's variant identifies the on-chain contracts and proving artifacts it was deployed with.
//
// One variant selects all the things that cannot diverge from one another for a pool:
//   - the private/JSON ABI that fungible transaction signatures are validated against (pkg/types/abis/IZetoFungible_V*.json)
//     and the Prepare-side calldata shape,
//   - the target-core interface ABIs used for event dispatch (internal/zeto/abis/IZeto*_V*.json),
//   - the circuit / proving-key tree used to produce proofs for the pool.
//
// A node proves every pool at a variant with that variant's single artifact tree, and a .zkey only works against the
// Groth16 verifier contract it was generated with. So every upstream release a variant covers must accept proofs from
// that tree and the same calldata shape; a release that changes either needs a new variant, even a patch release.
//
// Within a variant the private ABI may gain functions, provided their handlers work against every release the variant
// covers. Changing an existing function's signature needs a new variant, which may map to the same upstream release as
// an earlier one; existing pools keep the variant they were deployed at.
//
// Variant ordinals are Paladin's own, not upstream version numbers. The mapping is:
//
//	Variant    | zeto-contracts  | notes
//	-----------+-----------------+--------------------------------------------------------------------
//	V0         | v0.2.x (v0.2.2) | snake_case interface files; discrete proof tuple in Prepare calldata;
//	           |                 | lock() concatenates change outputs || lockedOutputs before verifyProof
//	V1         | v0.5.x (v0.5.1) | PascalCase interface files; single packed `bytes proof` calldata;
//	           |                 | _doLockTransition builds lockedOutputs || change outputs
//
// A pool's variant is recorded on chain in bytes 2-3 of its domain config header (see ZetoDomainConfigGen2), so it must
// not exceed ZetoDomainConfigMaxVariant (0xFFFF).
//
// When adding a variant, add the constant, extend SupportedZetoVariants, add its row above, and add the matching
// zetoVersions entry in domains/zeto/build.gradle.

// ZetoVariant identifies the on-chain contracts and proving artifacts a pool was deployed with.
type ZetoVariant uint64

const (
	// ZetoVariant_V0 is the zeto-contracts ~v0.2.x variant. Pools deployed before the variant was persisted decode as
	// 0, which is correct for them: they predate any later variant.
	ZetoVariant_V0 ZetoVariant = 0
	// ZetoVariant_V1 is the zeto-contracts ~v0.5.x variant.
	ZetoVariant_V1 ZetoVariant = 1
)

// SupportedZetoVariants is the authoritative list of accepted values. Every variant listed here must have a private ABI
// embedded under pkg/types/abis, target-core ABIs embedded under internal/zeto/abis and a proving-artifact tree named
// by ArtifactDirName().
var SupportedZetoVariants = []ZetoVariant{
	ZetoVariant_V0,
	ZetoVariant_V1,
}

// ArtifactDirName is the per-variant subdirectory holding this variant's circuits and proving keys, under the
// configured circuitsDir / provingKeysDir. A node serves every variant at once by holding one such directory each.
func (v ZetoVariant) ArtifactDirName() string { return v.String() }

func (v ZetoVariant) String() string { return fmt.Sprintf("V%d", uint64(v)) }

// ValidateZetoVariant rejects values this build does not know how to serve.
func ValidateZetoVariant(ctx context.Context, v ZetoVariant) error {
	for _, allowed := range SupportedZetoVariants {
		if v == allowed {
			return nil
		}
	}
	return i18n.NewError(ctx, msgs.MsgUnsupportedZetoVariant, v.String())
}

// ParseZetoVariantKey parses a domain config variant key of the form V<n>.
func ParseZetoVariantKey(ctx context.Context, key string) (ZetoVariant, error) {
	n, err := strconv.ParseUint(strings.TrimPrefix(key, "V"), 10, 64)
	if err != nil || ZetoVariant(n).String() != key {
		return 0, i18n.NewError(ctx, msgs.MsgInvalidZetoVariantKey, key)
	}
	v := ZetoVariant(n)
	return v, ValidateZetoVariant(ctx, v)
}

// UseZetoOnchainPackedProofCalldata is true when the target Zeto fungible token expects `transfer` / `deposit` / `withdraw`
// calldata with a single ABI-encoded `bytes proof` blob (Groth16 struct, optionally prefixed by root or encryption metadata),
// as implemented in upstream zeto solidity (~v0.5.x). V0 pools keep discrete proof tuple + public fields in Prepare.
func UseZetoOnchainPackedProofCalldata(variant ZetoVariant) bool {
	return variant != ZetoVariant_V0
}

// LockTransitionVerifierOutputOrderLockedFirst is true when the on-chain pool builds the ZK public-output vector as
// lockedOutputs || change outputs (ZetoFungible._doLockTransition in zeto-contracts ~v0.5+). Legacy ~v0.2.x tokens concatenate
// change outputs || lockedOutputs inside lock() before verifyProof; use ZetoVariant_V0 for that order.
func LockTransitionVerifierOutputOrderLockedFirst(variant ZetoVariant) bool {
	return variant == ZetoVariant_V1
}

// parseVersionOrdinal accepts either a JSON number or a string. The variant is declared as uint256 wherever it crosses
// an ABI boundary (the deploy constructor, the persisted domain config), and the ABI serializer renders uint256 as a
// decimal string — so a plain numeric Go field silently fails to decode a deploy that came through that path.
func parseVersionOrdinal(b []byte, field string) (uint64, error) {
	var n uint64
	if err := json.Unmarshal(b, &n); err == nil {
		return n, nil
	}
	var text string
	if err := json.Unmarshal(b, &text); err != nil {
		return 0, fmt.Errorf("%s must be a number or a numeric string: %s", field, string(b))
	}
	text = strings.TrimSpace(text)
	base := 10
	if lower := strings.ToLower(text); strings.HasPrefix(lower, "0x") {
		text, base = lower[2:], 16
	}
	v, err := strconv.ParseUint(text, base, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid version ordinal: %q", field, text)
	}
	return v, nil
}

// UnmarshalJSON accepts a number or a numeric string — see parseVersionOrdinal.
func (v *ZetoVariant) UnmarshalJSON(b []byte) error {
	n, err := parseVersionOrdinal(b, "variant")
	if err != nil {
		return err
	}
	*v = ZetoVariant(n)
	return nil
}

// MarshalJSON emits a number, matching how the variant is declared in Go-side configs.
func (v ZetoVariant) MarshalJSON() ([]byte, error) { return json.Marshal(uint64(v)) }
