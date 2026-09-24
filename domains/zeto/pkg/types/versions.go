/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the License); you may not use this file except in compliance with
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
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

// Paladin Zeto has two independent version axes. Both are recorded per pool in DomainInstanceConfig, and both must
// keep accepting every value they have ever written: the values are persisted on chain.
//
//	(1) ZetoFungibleABIVersion — Paladin's own transaction API generation. Selects the private/JSON ABI that fungible
//	    transaction signatures are validated against (pkg/types/abis/IZetoFungible_V*.json) and the Prepare-side
//	    calldata shape. Independent of upstream because Paladin can change its own request shape without a new zeto
//	    release. Persisted as DomainInstanceConfig.ZetoVariant (historical wire field name "zetoVariant").
//
//	(2) ZetoReleaseGeneration — which upstream zeto-contracts generation a pool was deployed from. One value selects
//	    all three things that are properties of that release and cannot diverge from one another:
//	      - the target-core interface ABIs used for event dispatch (internal/zeto/abis/IZeto*_V*.json),
//	      - the on-chain factory generation the pool was deployed through,
//	      - the circuit / proving-key tree used to produce proofs for the pool.
//	    Proving artifacts belong on this axis rather than an axis of their own: a pool's .zkey has to match the
//	    Groth16 verifier contract registered beside its token implementation, and both come from the same release.
//	    Persisted as DomainInstanceConfig.ReleaseGeneration, whose wire field name stays "factoryVersion" — the axis
//	    was introduced under that name and existing pools carry it.
//
// Generation ordinals are Paladin's own and are deliberately decoupled from upstream semver, so that a patch release
// upstream does not force a new generation here. The mapping is:
//
//	Generation | upstream zeto-contracts | notes
//	-----------+-------------------------+--------------------------------------------------------------------
//	V0         | ~v0.2.x (v0.2.2)        | snake_case interface files; discrete proof tuple in Prepare calldata;
//	           |                         | lock() concatenates change outputs || lockedOutputs before verifyProof
//	V1         | ~v0.5.x (v0.5.1)        | PascalCase interface files; single packed `bytes proof` calldata;
//	           |                         | _doLockTransition builds lockedOutputs || change outputs
//
// When adding a generation, add the constant, extend SupportedZetoReleaseGenerations, add its row above, and add the
// matching zetoVersions entry in domains/zeto/build.gradle.

// ZetoFungibleABIVersion selects pkg/types/abis/IZetoFungible_V*.json for fungible handler ABI validation.
// Defined type rather than an alias so that a raw HexUint64 cannot be passed by accident; use FungibleABIVersion()
// to convert at the persisted-config boundary.
type ZetoFungibleABIVersion pldtypes.HexUint64

const (
	// ZetoFungibleABI_V0 selects IZetoFungible_V0.json.
	ZetoFungibleABI_V0 ZetoFungibleABIVersion = 0
	// ZetoFungibleABI_V1 selects IZetoFungible_V1.json.
	ZetoFungibleABI_V1 ZetoFungibleABIVersion = 1
)

// SupportedZetoFungibleABIVersions is the authoritative list of accepted values; validation reads from it.
var SupportedZetoFungibleABIVersions = []ZetoFungibleABIVersion{
	ZetoFungibleABI_V0,
	ZetoFungibleABI_V1,
}

// FungibleABIVersion converts a persisted numeric zetoVariant into the typed axis value.
func FungibleABIVersion(v pldtypes.HexUint64) ZetoFungibleABIVersion {
	return ZetoFungibleABIVersion(v)
}

// Uint64 returns the persisted numeric form.
func (v ZetoFungibleABIVersion) Uint64() uint64 { return uint64(v) }

func (v ZetoFungibleABIVersion) String() string { return fmt.Sprintf("V%d", uint64(v)) }

// ValidateZetoFungibleABIVersion rejects values this build does not know how to serve.
func ValidateZetoFungibleABIVersion(ctx context.Context, v ZetoFungibleABIVersion) error {
	for _, allowed := range SupportedZetoFungibleABIVersions {
		if v == allowed {
			return nil
		}
	}
	return i18n.NewError(ctx, msgs.MsgUnsupportedZetoFungibleABIVersion, uint64(v))
}

// UseZetoOnchainPackedProofCalldata is true when the target Zeto fungible token expects `transfer` / `deposit` / `withdraw`
// calldata with a single ABI-encoded `bytes proof` blob (Groth16 struct, optionally prefixed by root or encryption metadata),
// as implemented in upstream zeto solidity (~v0.5.x). V0 Paladin configs keep discrete proof tuple + public fields in Prepare.
func UseZetoOnchainPackedProofCalldata(zetoVariant ZetoFungibleABIVersion) bool {
	return zetoVariant != ZetoFungibleABI_V0
}

// LockTransitionVerifierOutputOrderLockedFirst is true when the on-chain pool builds the ZK public-output vector as
// lockedOutputs || change outputs (ZetoFungible._doLockTransition in zeto-contracts ~v0.5+). Legacy ~v0.2.x tokens concatenate
// change outputs || lockedOutputs inside lock() before verifyProof; use ZetoFungibleABI_V0 / default deploy for that order.
func LockTransitionVerifierOutputOrderLockedFirst(zetoVariant ZetoFungibleABIVersion) bool {
	return zetoVariant == ZetoFungibleABI_V1
}

// ZetoReleaseGeneration identifies the upstream zeto-contracts generation a pool was deployed from. It selects the
// target-core ABIs, the factory generation, and the proving-artifact tree together — see the axis notes above.
type ZetoReleaseGeneration int64

const (
	// ZetoRelease_V0 is the zeto-contracts ~v0.2.x generation. Pools deployed before this axis was persisted decode
	// as 0, which is correct for them: they predate any later generation.
	ZetoRelease_V0 ZetoReleaseGeneration = 0
	// ZetoRelease_V1 is the zeto-contracts ~v0.5.x generation.
	ZetoRelease_V1 ZetoReleaseGeneration = 1
)

// SupportedZetoReleaseGenerations is the authoritative list of accepted values. Every generation listed here must
// have target-core ABIs embedded under internal/zeto/abis and a proving-artifact tree named by ArtifactDirName().
var SupportedZetoReleaseGenerations = []ZetoReleaseGeneration{
	ZetoRelease_V0,
	ZetoRelease_V1,
}

// zetoReleaseUpstreamTag documents which upstream release each generation was cut from. It is the single place the
// generation ordinal is tied to a zeto-contracts version; keep it aligned with zetoVersions in domains/zeto/build.gradle.
var zetoReleaseUpstreamTag = map[ZetoReleaseGeneration]string{
	ZetoRelease_V0: "v0.2.2",
	ZetoRelease_V1: "v0.5.1",
}

// UpstreamTag returns the zeto-contracts release this generation was cut from, e.g. "v0.5.1".
func (g ZetoReleaseGeneration) UpstreamTag() string { return zetoReleaseUpstreamTag[g] }

// ArtifactDirName is the per-generation subdirectory holding this generation's circuits and proving keys, under the
// configured circuitsDir / provingKeysDir. A node serves every generation at once by holding one such directory each.
func (g ZetoReleaseGeneration) ArtifactDirName() string { return g.String() }

func (g ZetoReleaseGeneration) String() string { return fmt.Sprintf("V%d", int64(g)) }

// ValidateZetoReleaseGeneration rejects values this build does not know how to serve.
func ValidateZetoReleaseGeneration(ctx context.Context, g ZetoReleaseGeneration) error {
	for _, allowed := range SupportedZetoReleaseGenerations {
		if g == allowed {
			return nil
		}
	}
	return i18n.NewError(ctx, msgs.MsgUnsupportedZetoReleaseGeneration, int64(g))
}

// parseVersionOrdinal accepts either a JSON number or a string. Both axes are declared as uint256 wherever they cross
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
func (v *ZetoFungibleABIVersion) UnmarshalJSON(b []byte) error {
	n, err := parseVersionOrdinal(b, "zetoVariant")
	if err != nil {
		return err
	}
	*v = ZetoFungibleABIVersion(n)
	return nil
}

// MarshalJSON emits a number, matching how the axis is declared in Go-side configs.
func (v ZetoFungibleABIVersion) MarshalJSON() ([]byte, error) { return json.Marshal(uint64(v)) }

// UnmarshalJSON accepts a number or a numeric string — see parseVersionOrdinal.
func (g *ZetoReleaseGeneration) UnmarshalJSON(b []byte) error {
	n, err := parseVersionOrdinal(b, "factoryVersion")
	if err != nil {
		return err
	}
	*g = ZetoReleaseGeneration(n)
	return nil
}

// MarshalJSON emits a number, matching how the axis is declared in Go-side configs.
func (g ZetoReleaseGeneration) MarshalJSON() ([]byte, error) { return json.Marshal(int64(g)) }
