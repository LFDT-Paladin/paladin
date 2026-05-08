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

	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

//go:embed abis/IZeto.json
var zetoABIBytes []byte

//go:embed abis/IZeto_V1.json
var zetoABIBytesV1 []byte

//go:embed abis/IZetoLockable.json
var zetoLockableABIBytes []byte

//go:embed abis/IZetoKyc.json
var zetoKycABIBytes []byte

func mergeZetoCoreEvents(core *solutils.SolidityBuild) abi.ABI {
	var events abi.ABI
	events = appendEventsFromBuild(events, core)
	contract := solutils.MustLoadBuild(zetoLockableABIBytes)
	events = appendEventsFromBuild(events, contract)
	contract = solutils.MustLoadBuild(zetoKycABIBytes)
	events = appendEventsFromBuild(events, contract)
	return dedupEvents(events)
}

// zetoEventABISet returns merged events for one core interface generation (Phase B).
func zetoEventABISet(useV1Core bool) abi.ABI {
	var core []byte
	if useV1Core {
		core = zetoABIBytesV1
	} else {
		core = zetoABIBytes
	}
	return mergeZetoCoreEvents(solutils.MustLoadBuild(core))
}

// getAllZetoEventAbis merges events from all supported generations for ConfigureDomain (Paladin ABI index).
func getAllZetoEventAbis() abi.ABI {
	var events abi.ABI
	events = append(events, zetoEventABISet(false)...)
	events = append(events, zetoEventABISet(true)...)
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
