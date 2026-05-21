/*
 * Copyright © 2026 Kaleido, Inc.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

const (
	zetoFactoryV1ABIFile = "ZetoFactoryV1.json"
	erc1967ProxyABIFile   = "ERC1967Proxy.json"
)

func isZetoFactoryV1Deploy(factory *zetoDomainContract) bool {
	return strings.EqualFold(filepath.Base(factory.AbiAndBytecode.Path), zetoFactoryV1ABIFile)
}

// deployZetoFactoryV1WithProxy mirrors upgrades.deployProxy(ZetoFactoryV1, [], { initializer: "initialize" }):
//  1. deploy ZetoFactoryV1 implementation (inherits ZetoTokenFactoryUpgradeable; constructor disables initializers)
//  2. deploy ERC1967Proxy(implementation, initialize calldata)
//  3. return proxy address — the address used for registerImplementation / domain registration
func deployZetoFactoryV1WithProxy(
	ctx context.Context,
	rpc rpcclient.Client,
	deployer string,
	factoryBuild *solutils.SolidityBuild,
) (*pldtypes.EthAddress, abi.ABI, error) {
	implAddr, err := deployBytecode(ctx, rpc, deployer, factoryBuild)
	if err != nil {
		return nil, nil, fmt.Errorf("deploy ZetoFactoryV1 implementation: %w", err)
	}
	log.L(ctx).Infof("Deployed ZetoFactoryV1 implementation at %s", implAddr)

	initCalldata, err := encodeUpgradeableFactoryInitializeCalldata(factoryBuild.ABI)
	if err != nil {
		return nil, nil, err
	}

	proxyBuild, err := loadSolidityBuildFromPath(filepath.Join("helpers", "abis", erc1967ProxyABIFile))
	if err != nil {
		return nil, nil, err
	}
	proxyParams, err := json.Marshal(map[string]string{
		"implementation": implAddr.String(),
		"_data":          initCalldata,
	})
	if err != nil {
		return nil, nil, err
	}
	proxyAddr, err := deployBytecodeWithParams(ctx, rpc, deployer, proxyBuild, pldtypes.RawJSON(proxyParams))
	if err != nil {
		return nil, nil, fmt.Errorf("deploy ERC1967Proxy for ZetoFactoryV1: %w", err)
	}
	log.L(ctx).Infof("Deployed ZetoFactoryV1 proxy at %s (initialize in proxy constructor)", proxyAddr)

	if err := verifyUpgradeableFactoryOwner(ctx, rpc, deployer, proxyAddr, factoryBuild.ABI); err != nil {
		return nil, nil, err
	}
	return proxyAddr, factoryBuild.ABI, nil
}

func loadSolidityBuildFromPath(path string) (*solutils.SolidityBuild, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read abi+bytecode %s: %w", path, err)
	}
	return solutils.MustLoadBuild(bytes), nil
}

func encodeUpgradeableFactoryInitializeCalldata(factoryABI abi.ABI) (string, error) {
	initFn := factoryABI.Functions()["initialize"]
	if initFn == nil {
		return "", fmt.Errorf("factory ABI missing initialize()")
	}
	data, err := initFn.EncodeCallDataJSON(pldtypes.JSONString([]any{}))
	if err != nil {
		return "", fmt.Errorf("encode initialize calldata: %w", err)
	}
	return pldtypes.HexBytes(data).String(), nil
}

func verifyUpgradeableFactoryOwner(ctx context.Context, rpc rpcclient.Client, deployer string, proxyAddr *pldtypes.EthAddress, factoryABI abi.ABI) error {
	deployerEth, err := resolveDeployerEthAddress(ctx, rpc, deployer)
	if err != nil {
		return err
	}
	ownerFn := factoryABI.Functions()["owner"]
	if ownerFn == nil {
		return fmt.Errorf("factory ABI missing owner()")
	}
	ownerJSON, err := ethCallRead(ctx, rpc, deployer, proxyAddr, ownerFn)
	if err != nil {
		return fmt.Errorf("read factory owner: %w", err)
	}
	ownerAddr, err := decodeEthAddressFromCallResult(ownerJSON)
	if err != nil {
		return fmt.Errorf("decode factory owner: %w", err)
	}
	if ownerAddr == nil || ownerAddr.String() != deployerEth.String() {
		return fmt.Errorf("factory proxy owner %s does not match deployer %s (expected upgrades.deployProxy initializer owner)", ownerAddr, deployerEth)
	}
	log.L(ctx).Infof("Verified ZetoFactoryV1 proxy owner is deployer %s", deployerEth)
	return nil
}

func resolveDeployerEthAddress(ctx context.Context, rpc rpcclient.Client, deployer string) (*pldtypes.EthAddress, error) {
	var addr pldtypes.HexBytes
	rpcerr := rpc.CallRPC(ctx, &addr, "ptx_resolveVerifier", deployer, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return pldtypes.MustEthAddress(addr.String()), nil
}

func ethCallRead(ctx context.Context, rpc rpcclient.Client, from string, to *pldtypes.EthAddress, fn *abi.Entry) (pldtypes.RawJSON, error) {
	// ptx_call expects JSON-encoded inputs (not ABI calldata); see sdk/go/pkg/pldclient/txbuilder_test.go.
	var result pldtypes.RawJSON
	rpcerr := rpc.CallRPC(ctx, &result, "ptx_call", &pldapi.TransactionCall{
		TransactionInput: pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type:     pldapi.TransactionTypePublic.Enum(),
				From:     from,
				To:       to,
				Function: fn.String(),
				Data:     pldtypes.JSONString([]any{}),
			},
			ABI: abi.ABI{fn},
		},
	})
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return result, nil
}

func decodeEthAddressFromCallResult(raw pldtypes.RawJSON) (*pldtypes.EthAddress, error) {
	var owner string
	if err := json.Unmarshal(raw, &owner); err == nil {
		return pldtypes.MustEthAddress(normalizeEthAddressHex(owner)), nil
	}
	var indexed map[string]string
	if err := json.Unmarshal(raw, &indexed); err == nil {
		if v, ok := indexed["0"]; ok {
			return pldtypes.MustEthAddress(normalizeEthAddressHex(v)), nil
		}
	}
	var tuple []string
	if err := json.Unmarshal(raw, &tuple); err == nil && len(tuple) > 0 {
		return pldtypes.MustEthAddress(normalizeEthAddressHex(tuple[0])), nil
	}
	return nil, fmt.Errorf("decode eth address from call result %s", string(raw))
}

func normalizeEthAddressHex(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return "0x" + s
	}
	return s
}

func deployBytecodeWithParams(ctx context.Context, rpc rpcclient.Client, deployer string, build *solutils.SolidityBuild, params pldtypes.RawJSON) (*pldtypes.EthAddress, error) {
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode", deployer, build.ABI, build.Bytecode.String(), params)
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return pldtypes.MustEthAddress(addr), nil
}
