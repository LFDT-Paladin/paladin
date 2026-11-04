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

package zeto

import (
	_ "embed"
	"fmt"

	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/hyperledger-firefly/signer/pkg/abi"
)

//go:embed abis/IZeto_V0.json
var zetoCoreJSON_V0 []byte

//go:embed abis/IZeto_V1.json
var zetoCoreJSON_V1 []byte

//go:embed abis/IZetoLockable_V0.json
var zetoLockableJSON_V0 []byte

// The V1 lockable ABI comes from upstream ILockableCapability.sol; Gradle copies it under the Paladin-side role name
// so that IZetoLockable_V0.json / IZetoLockable_V1.json form a matching pair.
//
//go:embed abis/IZetoLockable_V1.json
var zetoLockableJSON_V1 []byte

//go:embed abis/IZetoKyc_V0.json
var zetoKycJSON_V0 []byte

//go:embed abis/IZetoKyc_V1.json
var zetoKycJSON_V1 []byte

// zetoTargetCoreABIs holds the target-core interface ABIs for each ZetoVariant. Adding a variant means adding the three
// embeds and one row here — the merge and dispatch below are variant-agnostic.
var zetoTargetCoreABIs = map[types.ZetoVariant]struct{ core, lockable, kyc []byte }{
	types.ZetoVariant_V0: {core: zetoCoreJSON_V0, lockable: zetoLockableJSON_V0, kyc: zetoKycJSON_V0},
	types.ZetoVariant_V1: {core: zetoCoreJSON_V1, lockable: zetoLockableJSON_V1, kyc: zetoKycJSON_V1},
}

func mergeZetoCoreEvents(core *solutils.SolidityBuild, lockableJSON, kycJSON []byte) abi.ABI {
	var events abi.ABI
	events = appendEventsFromBuild(events, core)
	events = appendEventsFromBuild(events, solutils.MustLoadBuild(lockableJSON))
	events = appendEventsFromBuild(events, solutils.MustLoadBuild(kycJSON))
	return dedupEvents(events)
}

// zetoEventABISet returns merged events for one ZetoVariant.
func zetoEventABISet(v types.ZetoVariant) abi.ABI {
	set, ok := zetoTargetCoreABIs[v]
	if !ok {
		panic(fmt.Sprintf("unsupported zeto variant: %s", v))
	}
	return mergeZetoCoreEvents(solutils.MustLoadBuild(set.core), set.lockable, set.kyc)
}

// getAllZetoEventAbis merges events from all supported variants for ConfigureDomain (Paladin ABI index).
func getAllZetoEventAbis() abi.ABI {
	var events abi.ABI
	for _, v := range types.SupportedZetoVariants {
		events = append(events, zetoEventABISet(v)...)
	}
	return dedupEvents(events)
}

func appendEventsFromBuild(events abi.ABI, contract *solutils.SolidityBuild) abi.ABI {
	for _, entry := range contract.ABI {
		if entry.Type == abi.Event {
			events = append(events, entry)
		}
	}
	return events
}

func dedupEvents(events abi.ABI) abi.ABI {
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			if events[i].Name == events[j].Name {
				events = append(events[:j], events[j+1:]...)
				j--
			}
		}
	}
	return events
}
