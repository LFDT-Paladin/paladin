// Copyright © 2024 Kaleido, Inc.
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

package keymanager

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/rpcserver"
)

func (km *keyManager) RPCModule() *rpcserver.RPCModule {
	return km.rpcModule
}

func (km *keyManager) initRPC() {
	km.rpcModule = rpcserver.NewRPCModule("keymgr").
		Add("keymgr_wallets", km.rpcWallets()).
		Add("keymgr_resolveKey", km.rpcResolveKey()).
		Add("keymgr_resolveEthAddress", km.rpcResolveEthAddress()).
		Add("keymgr_reverseKeyLookup", km.rpcReverseKeyLookup()).
		Add("keymgr_queryKeys", km.rpcQueryKeys())

	if !km.conf.DisableSignRPC {
		km.rpcModule.Add("keymgr_sign", km.rpcSign())
	}
}

func (km *keyManager) rpcWallets() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context,
	) ([]*pldapi.WalletInfo, error) {
		return km.getWalletList(), nil
	})
}

func (km *keyManager) rpcResolveKey() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		identifier string,
		algorithm string,
		verifierType string,
	) (*pldapi.KeyMappingAndVerifier, error) {
		ctx = log.WithComponent(ctx, "keymanager")
		return km.ResolveKeyNewDatabaseTX(ctx, identifier, algorithm, verifierType)
	})
}

func (km *keyManager) rpcResolveEthAddress() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		identifier string,
	) (*pldtypes.EthAddress, error) {
		ctx = log.WithComponent(ctx, "keymanager")
		return km.ResolveEthAddressNewDatabaseTX(ctx, identifier)
	})
}

func (km *keyManager) rpcReverseKeyLookup() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		algorithm string,
		verifierType string,
		verifier string,
	) (*pldapi.KeyMappingAndVerifier, error) {
		ctx = log.WithComponent(ctx, "keymanager")
		return km.ReverseKeyLookup(ctx, km.p.NOTX(), algorithm, verifierType, verifier)
	})
}

func (km *keyManager) rpcQueryKeys() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		jq query.QueryJSON,
	) ([]*pldapi.KeyQueryEntry, error) {
		ctx = log.WithComponent(ctx, "keymanager")
		return km.QueryKeys(ctx, km.p.DB(), &jq)
	})
}

// rpcSign signs arbitrary data using the specified algorithm and verifier type.
//
// Example:
//   - algorithm: algorithms.ECDSA_SECP256K1
//   - verifierType: verifiers.ETH_ADDRESS
//   - payloadType: signpayloads.OPAQUE_TO_RSV
//
// Default handling:
//   - The payload is hashed with keccak256 internally by the signing module before signing
//   - Returns a 65-byte RSV signature (R=32bytes, S=32bytes, V=1byte with 0/1 recovery ID)
func (km *keyManager) rpcSign() rpcserver.RPCHandler {
	return rpcserver.RPCMethod5(func(ctx context.Context,
		identifier string,
		algorithm string,
		verifierType string,
		payloadType string,
		payload pldtypes.HexBytes,
	) (pldtypes.HexBytes, error) {
		ctx = log.WithComponent(ctx, "keymanager")
		resolvedKey, err := km.ResolveKeyNewDatabaseTX(ctx, identifier, algorithm, verifierType)
		if err != nil {
			return nil, err
		}
		signature, err := km.Sign(ctx, resolvedKey, payloadType, payload)
		if err != nil {
			return nil, err
		}
		return signature, nil
	})
}
