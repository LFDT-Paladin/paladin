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
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

//go:embed abis/IZeto.json
var zetoABIBytes []byte

//go:embed abis/IZeto_V1.json
var zetoABIBytesV1 []byte

//go:embed abis/IZetoLockable.json
var zetoLockableABIBytes []byte

// Zeto v0.5.x lockable events/calls come from ILockableCapability (Gradle copies it as IZetoLockableCapability_V1.json).
//
//go:embed abis/IZetoLockableCapability_V1.json
var zetoLockableABIBytesV1 []byte

//go:embed abis/IZetoKyc.json
var zetoKycABIBytes []byte

func mergeZetoCoreEvents(core *solutils.SolidityBuild, lockableJSON []byte) abi.ABI {
	var events abi.ABI
	events = appendEventsFromBuild(events, core)
	contract := solutils.MustLoadBuild(lockableJSON)
	events = appendEventsFromBuild(events, contract)
	contract = solutils.MustLoadBuild(zetoKycABIBytes)
	events = appendEventsFromBuild(events, contract)
	return dedupEvents(events)
}

func zetoCoreABIBytes(v types.ZetoTargetContractABIVersion) []byte {
	switch v {
	case types.ZetoTargetContractABI_V0:
		return zetoABIBytes
	case types.ZetoTargetContractABI_V1:
		return zetoABIBytesV1
	default:
		panic(fmt.Sprintf("unsupported zeto core ABI version: %d", v))
	}
}

// zetoEventABISet returns merged events for one ZetoTargetContractABIVersion (axis 2; IZeto*.json).
func zetoEventABISet(v types.ZetoTargetContractABIVersion) abi.ABI {
	core := zetoCoreABIBytes(v)
	lockable := zetoLockableABIBytes
	if v == types.ZetoTargetContractABI_V1 {
		lockable = zetoLockableABIBytesV1
	}
	return mergeZetoCoreEvents(solutils.MustLoadBuild(core), lockable)
}

// getAllZetoEventAbis merges events from all supported generations for ConfigureDomain (Paladin ABI index).
func getAllZetoEventAbis() abi.ABI {
	var events abi.ABI
	for _, v := range types.SupportedZetoTargetContractABIVersions {
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
