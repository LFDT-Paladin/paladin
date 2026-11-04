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

package helpers

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"testing"

	"github.com/go-resty/resty/v2"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/pkg/testbed"
	zetotypes "github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/hyperledger-firefly/signer/pkg/abi"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

//go:embed abis/ZetoFactory_V1.json
var zetoFactoryJSON []byte

type ZetoDomainConfig struct {
	DomainContracts zetoDomainContracts `yaml:"contracts"`
}

// zetoVariantsDeployConfig deploys one factory holding the implementations of several variants. Each variant's
// contracts are deployed and resolved separately, so contract names may repeat across variants; implementations whose
// token name repeats need distinct registeredNames on the factory.
type zetoVariantsDeployConfig struct {
	Factory  zetoDomainContract                    `yaml:"factory"`
	Variants map[string]zetoVariantDeployContracts `yaml:"variants"`
}

type zetoVariantDeployContracts struct {
	Implementations []zetoDomainContract `yaml:"implementations"`
}

type ZetoDomainContracts struct {
	FactoryAddress       *pldtypes.EthAddress
	factoryAbi           abi.ABI
	deployedContracts    map[string]*pldtypes.EthAddress
	DeployedContractAbis map[string]abi.ABI
	cloneableContracts   map[string]cloneableContract
}

type cloneableContract struct {
	registeredName        string
	circuits              *zetosignerapi.Circuits
	verifier              string
	batchVerifier         string
	depositVerifier       string
	withdrawVerifier      string
	batchWithdrawVerifier string
	lockVerifier          string
	batchLockVerifier     string
	burnVerifier          string
	batchBurnVerifier     string
}

type zetoDomainContracts struct {
	Factory         zetoDomainContract   `yaml:"factory"`
	Implementations []zetoDomainContract `yaml:"implementations"`
}

type zetoDomainContract struct {
	Name                  string                  `yaml:"name"`
	RegisteredName        string                  `yaml:"registeredName,omitempty"`
	Verifier              string                  `yaml:"verifier"`
	BatchVerifier         string                  `yaml:"batchVerifier"`
	DepositVerifier       string                  `yaml:"depositVerifier"`
	WithdrawVerifier      string                  `yaml:"withdrawVerifier"`
	BatchWithdrawVerifier string                  `yaml:"batchWithdrawVerifier"`
	LockVerifier          string                  `yaml:"lockVerifier"`
	BatchLockVerifier     string                  `yaml:"batchLockVerifier"`
	BurnVerifier          string                  `yaml:"burnVerifier,omitempty"`
	BatchBurnVerifier     string                  `yaml:"batchBurnVerifier,omitempty"`
	Circuits              *zetosignerapi.Circuits `yaml:"circuits"`
	AbiAndBytecode        abiAndBytecode          `yaml:"abiAndBytecode"`
	Libraries             []string                `yaml:"libraries"`
	Cloneable             bool                    `yaml:"cloneable"`
}

type abiAndBytecode struct {
	Path string `yaml:"path"`
}

type setImplementationParams struct {
	Name           string             `json:"name"`
	Implementation implementationInfo `json:"implementation"`
}

type implementationInfo struct {
	Implementation string        `json:"implementation"`
	Verifiers      verifiersInfo `json:"verifiers"`
}

type verifiersInfo struct {
	Verifier              string `json:"verifier"`
	BatchVerifier         string `json:"batchVerifier"`
	DepositVerifier       string `json:"depositVerifier"`
	WithdrawVerifier      string `json:"withdrawVerifier"`
	BatchWithdrawVerifier string `json:"batchWithdrawVerifier"`
	LockVerifier          string `json:"lockVerifier"`
	BatchLockVerifier     string `json:"batchLockVerifier"`
	BurnVerifier          string `json:"burnVerifier"`
	BatchBurnVerifier     string `json:"batchBurnVerifier"`
}

func DeployZetoContracts(t *testing.T, hdWalletSeed *testbed.UTInitFunction, configFile string, controller string, variant zetotypes.ZetoVariant) *ZetoDomainContracts {
	config, err := loadYAML[ZetoDomainConfig](configFile)
	require.NoError(t, err)
	deployed := deployZetoContractsFromConfig(t, hdWalletSeed, controller, config.DomainContracts.Factory, map[zetotypes.ZetoVariant][]zetoDomainContract{
		variant: config.DomainContracts.Implementations,
	})
	return deployed[variant]
}

// DeployZetoVariantContracts deploys a zetoVariantsDeployConfig, returning each variant's contracts. All of them share
// one factory.
func DeployZetoVariantContracts(t *testing.T, hdWalletSeed *testbed.UTInitFunction, configFile string, controller string) map[zetotypes.ZetoVariant]*ZetoDomainContracts {
	config, err := loadYAML[zetoVariantsDeployConfig](configFile)
	require.NoError(t, err)
	variants := make(map[zetotypes.ZetoVariant][]zetoDomainContract, len(config.Variants))
	for key, vc := range config.Variants {
		v, err := zetotypes.ParseZetoVariantKey(t.Context(), key)
		require.NoError(t, err)
		variants[v] = vc.Implementations
	}
	return deployZetoContractsFromConfig(t, hdWalletSeed, controller, config.Factory, variants)
}

func loadYAML[T any](configFile string) (*T, error) {
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var config T
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func deployZetoContractsFromConfig(t *testing.T, hdWalletSeed *testbed.UTInitFunction, controller string, factory zetoDomainContract, variants map[zetotypes.ZetoVariant][]zetoDomainContract) map[zetotypes.ZetoVariant]*ZetoDomainContracts {
	ctx := context.Background()
	log.L(ctx).Infof("Deploy Zeto Contracts")

	tb := testbed.NewTestBed()
	httpURL, _, _, done, err := tb.StartForTest("./testbed.config.yaml", map[string]*testbed.TestbedDomain{}, hdWalletSeed)
	require.NoError(t, err)
	defer done()
	rpc := rpcclient.WrapRestyClient(resty.New().SetBaseURL(httpURL))

	// deploy the factory contract
	factoryAddr, factoryABI, err := deployContract(ctx, rpc, controller, &factory, nil)
	require.NoError(t, err)

	// deployContract already wrapped the upgradeable ZetoFactory_V1 implementation in its ERC1967Proxy and returned the
	// proxy address; that proxy is what registerImplementation and domain registration use.
	log.L(ctx).Infof("Using ZetoFactory_V1 proxy at %s", factoryAddr.String())

	deployed := make(map[zetotypes.ZetoVariant]*ZetoDomainContracts, len(variants))
	for variant, implementations := range variants {
		require.NotEmpty(t, implementations, "no implementations specified for variant %s", variant)
		for i := range implementations {
			implementations[i].AbiAndBytecode.Path = ResolveZetoImplementationAbiPath(implementations[i].AbiAndBytecode.Path, variant)
		}
		// deploy the implementation contracts
		deployedContracts, deployedContractAbis, err := deployImplementations(ctx, rpc, controller, implementations)
		require.NoError(t, err)

		ctrs := newZetoDomainContracts()
		ctrs.factoryAbi = factoryABI
		ctrs.FactoryAddress = factoryAddr
		ctrs.deployedContracts = deployedContracts
		ctrs.DeployedContractAbis = deployedContractAbis
		// the cloneable contracts are the ones that can be cloned by the factory
		// these are the top level Zeto token contracts
		ctrs.cloneableContracts = findCloneableContracts(implementations)
		require.NoError(t, configureFactoryContract(ctx, tb, controller, ctrs))
		deployed[variant] = ctrs
	}
	return deployed
}

// PrepareZetoConfig builds a domain config with domainContracts, which serves the V0 variant only from artifacts laid
// out flat.
func PrepareZetoConfig(t *testing.T, domainContracts *ZetoDomainContracts) *zetotypes.DomainFactoryConfig {
	zkpDir := ZetoZKArtifactsDir(zetotypes.ZetoVariant_V0)
	return &zetotypes.DomainFactoryConfig{
		DomainContracts: &zetotypes.DomainConfigContracts{
			Implementations: domainContracts.domainConfigImplementations(),
		},
		SnarkProver: zetosignerapi.SnarkProverConfig{
			CircuitsDir:    zkpDir,
			ProvingKeysDir: zkpDir,
		},
	}
}

// PrepareZetoVariantConfig builds a domain config with variants.
func PrepareZetoVariantConfig(t *testing.T, defaultVariant zetotypes.ZetoVariant, domainContracts map[zetotypes.ZetoVariant]*ZetoDomainContracts) *zetotypes.DomainFactoryConfig {
	config := &zetotypes.DomainFactoryConfig{
		Variants:       make(map[string]*zetotypes.VariantConfig, len(domainContracts)),
		DefaultVariant: defaultVariant.String(),
	}
	var variants []zetotypes.ZetoVariant
	for variant, contracts := range domainContracts {
		config.Variants[variant.String()] = &zetotypes.VariantConfig{
			Implementations: contracts.domainConfigImplementations(),
		}
		variants = append(variants, variant)
	}
	zkpDir := ZetoVariantArtifactsDir(t, variants...)
	config.SnarkProver = zetosignerapi.SnarkProverConfig{
		CircuitsDir:    zkpDir,
		ProvingKeysDir: zkpDir,
	}
	return config
}

func (dc *ZetoDomainContracts) domainConfigImplementations() []*zetotypes.DomainContract {
	var impls []*zetotypes.DomainContract
	for name, implContract := range dc.cloneableContracts {
		impls = append(impls, &zetotypes.DomainContract{
			Name:           name,
			RegisteredName: implContract.registeredName,
			Circuits:       implContract.circuits,
		})
	}
	return impls
}

func newZetoDomainContracts() *ZetoDomainContracts {
	factory := solutils.MustLoadBuild(zetoFactoryJSON)

	return &ZetoDomainContracts{
		factoryAbi: factory.ABI,
	}
}

func findCloneableContracts(implementations []zetoDomainContract) map[string]cloneableContract {
	cloneableContracts := make(map[string]cloneableContract)
	for _, contract := range implementations {
		if contract.Cloneable {
			cloneableContracts[contract.Name] = cloneableContract{
				registeredName:        contract.RegisteredName,
				circuits:              contract.Circuits,
				verifier:              contract.Verifier,
				batchVerifier:         contract.BatchVerifier,
				depositVerifier:       contract.DepositVerifier,
				withdrawVerifier:      contract.WithdrawVerifier,
				batchWithdrawVerifier: contract.BatchWithdrawVerifier,
				lockVerifier:          contract.LockVerifier,
				batchLockVerifier:     contract.BatchLockVerifier,
				burnVerifier:          contract.BurnVerifier,
				batchBurnVerifier:     contract.BatchBurnVerifier,
			}
		}
	}
	return cloneableContracts
}

func deployImplementations(ctx context.Context, rpc rpcclient.Client, deployer string, contracts []zetoDomainContract) (map[string]*pldtypes.EthAddress, map[string]abi.ABI, error) {
	deployedContracts := make(map[string]*pldtypes.EthAddress)
	deployedContractAbis := make(map[string]abi.ABI)
	for _, contract := range contracts {
		addr, abi, err := deployContract(ctx, rpc, deployer, &contract, deployedContracts)
		if err != nil {
			return nil, nil, err
		}
		log.L(ctx).Infof("Deployed contract %s to %s", contract.Name, addr.String())
		deployedContracts[contract.Name] = addr
		deployedContractAbis[contract.Name] = abi
	}

	return deployedContracts, deployedContractAbis, nil
}

func deployContract(ctx context.Context, rpc rpcclient.Client, deployer string, contract *zetoDomainContract, deployedContracts map[string]*pldtypes.EthAddress) (*pldtypes.EthAddress, abi.ABI, error) {
	if contract.AbiAndBytecode.Path == "" {
		return nil, nil, fmt.Errorf("no path or JSON specified for the abi and bytecode for contract %s", contract.Name)
	}
	build, err := getContractSpec(contract, deployedContracts)
	if err != nil {
		return nil, nil, err
	}
	if isZetoFactoryDeploy(contract) {
		return deployZetoFactoryWithProxy(ctx, rpc, deployer, build)
	}
	addr, err := deployBytecode(ctx, rpc, deployer, build)
	if err != nil {
		return nil, nil, err
	}
	return addr, build.ABI, nil
}

func getContractSpec(contract *zetoDomainContract, deployedContracts map[string]*pldtypes.EthAddress) (*solutils.SolidityBuild, error) {
	bytes, err := os.ReadFile(contract.AbiAndBytecode.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read abi+bytecode file %s. %s", contract.AbiAndBytecode.Path, err)
	}
	build := solutils.MustLoadBuildResolveLinks(bytes, deployedContracts)
	return build, nil
}

func deployBytecode(ctx context.Context, rpc rpcclient.Client, deployer string, build *solutils.SolidityBuild) (*pldtypes.EthAddress, error) {
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode", deployer, build.ABI, build.Bytecode.String(), pldtypes.RawJSON(`{}`))
	if rpcerr != nil {
		return nil, rpcerr
	}
	return pldtypes.MustEthAddress(addr), nil
}

func configureFactoryContract(ctx context.Context, tb testbed.Testbed, deployer string, domainContracts *ZetoDomainContracts) error {
	abiFunc := domainContracts.factoryAbi.Functions()["registerImplementation"]

	// Send the transaction
	for contractName := range domainContracts.cloneableContracts {
		err := registerImpl(ctx, contractName, domainContracts, abiFunc, deployer, domainContracts.FactoryAddress, tb)
		if err != nil {
			return err
		}
	}

	return nil
}

func registerImpl(ctx context.Context, name string, domainContracts *ZetoDomainContracts, abiFunc *abi.Entry, deployer string, addr *pldtypes.EthAddress, tb testbed.Testbed) error {
	if name == "" {
		return fmt.Errorf("no name specified for implementation")
	}

	log.L(ctx).Infof("Registering implementation %s", name)
	verifierName := domainContracts.cloneableContracts[name].verifier
	batchVerifierName := domainContracts.cloneableContracts[name].batchVerifier
	depositVerifierName := domainContracts.cloneableContracts[name].depositVerifier
	withdrawVerifierName := domainContracts.cloneableContracts[name].withdrawVerifier
	batchWithdrawVerifierName := domainContracts.cloneableContracts[name].batchWithdrawVerifier
	lockVerifierName := domainContracts.cloneableContracts[name].lockVerifier
	batchLockVerifierName := domainContracts.cloneableContracts[name].batchLockVerifier
	burnVerifierName := domainContracts.cloneableContracts[name].burnVerifier
	batchBurnVerifierName := domainContracts.cloneableContracts[name].batchBurnVerifier

	registeredName := domainContracts.cloneableContracts[name].registeredName
	if registeredName == "" {
		registeredName = name
	}
	params := &setImplementationParams{
		Name: registeredName,
	}

	if verifierName == "" {
		return fmt.Errorf("verifierName not found among the deployed contracts. name: %s", name)
	}

	implAddr, ok := domainContracts.deployedContracts[name]
	if !ok {
		return fmt.Errorf("implementation contract %s not found among the deployed contracts", name)
	}
	params.Implementation.Implementation = implAddr.String()

	verifierAddr, ok := domainContracts.deployedContracts[verifierName]
	if !ok {
		return fmt.Errorf("verifier contract %s not found among the deployed contracts", verifierName)
	}
	params.Implementation.Verifiers.Verifier = verifierAddr.String()
	if params.Implementation.Verifiers.Verifier == "" {
		return fmt.Errorf("resolved verifier address for %s is empty", verifierName)
	}

	if batchVerifierName != "" {
		batchVerifierAddr, ok := domainContracts.deployedContracts[batchVerifierName]
		if !ok {
			return fmt.Errorf("batch verifier contract %s not found among the deployed contracts", batchVerifierName)
		}
		params.Implementation.Verifiers.BatchVerifier = batchVerifierAddr.String()
	}

	if depositVerifierName != "" {
		depositVerifierAddr, ok := domainContracts.deployedContracts[depositVerifierName]
		if !ok {
			return fmt.Errorf("deposit verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.DepositVerifier = depositVerifierAddr.String()
	}

	if withdrawVerifierName != "" {
		withdrawVerifierAddr, ok := domainContracts.deployedContracts[withdrawVerifierName]
		if !ok {
			return fmt.Errorf("withdraw verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.WithdrawVerifier = withdrawVerifierAddr.String()
	}

	if batchWithdrawVerifierName != "" {
		batchWithdrawVerifierAddr, ok := domainContracts.deployedContracts[batchWithdrawVerifierName]
		if !ok {
			return fmt.Errorf("batch withdraw verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.BatchWithdrawVerifier = batchWithdrawVerifierAddr.String()
	}

	if lockVerifierName != "" {
		lockVerifierAddr, ok := domainContracts.deployedContracts[lockVerifierName]
		if !ok {
			return fmt.Errorf("lock verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.LockVerifier = lockVerifierAddr.String()
	}

	if batchLockVerifierName != "" {
		batchLockVerifierAddr, ok := domainContracts.deployedContracts[batchLockVerifierName]
		if !ok {
			return fmt.Errorf("batch lock verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.BatchLockVerifier = batchLockVerifierAddr.String()
	}

	if burnVerifierName != "" {
		burnVerifierAddr, ok := domainContracts.deployedContracts[burnVerifierName]
		if !ok {
			return fmt.Errorf("lock verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.BurnVerifier = burnVerifierAddr.String()
	}

	if batchBurnVerifierName != "" {
		batchBurnVerifierAddr, ok := domainContracts.deployedContracts[batchBurnVerifierName]
		if !ok {
			return fmt.Errorf("batch lock verifier contract not found among the deployed contracts")
		}
		params.Implementation.Verifiers.BatchBurnVerifier = batchBurnVerifierAddr.String()
	}

	// Match zeto/solidity/scripts/tokens/Zeto_Anon.ts (and Hardhat tests): non-burnable tokens still pass explicit
	// zero addresses for burn verifiers so IZetoInitializable.VerifiersInfo matches the factory ABI tuple.
	const zeroVerifierAddress = "0x0000000000000000000000000000000000000000"
	if params.Implementation.Verifiers.BurnVerifier == "" {
		params.Implementation.Verifiers.BurnVerifier = zeroVerifierAddress
	}
	if params.Implementation.Verifiers.BatchBurnVerifier == "" {
		params.Implementation.Verifiers.BatchBurnVerifier = zeroVerifierAddress
	}

	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     deployer,
			To:       addr,
			Data:     pldtypes.JSONString(params),
			Function: abiFunc.String(),
		},
		ABI: abi.ABI{abiFunc},
	})
	log.L(ctx).Infof("Registered implementation %s as %s", name, registeredName)
	return err
}
