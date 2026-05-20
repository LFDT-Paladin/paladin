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
	"context"
	_ "embed"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/common"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/fungible"
	"github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/nonfungible"
	signercommon "github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/signer/common"
	zetosmt "github.com/LFDT-Paladin/paladin/domains/zeto/internal/zeto/smt"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/types"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LFDT-Paladin/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/signerapi"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/smt"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/LFDT-Paladin/smt/pkg/utxo"
	"github.com/LFDT-Paladin/smt/pkg/utxo/core"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/iden3/go-iden3-crypto/babyjub"
)

var _ plugintk.DomainAPI = &Zeto{}

//go:embed factoryabis/ZetoFactory.json
var zetoFactoryArtifactV0 []byte

//go:embed factoryabis/ZetoFactoryV1.json
var zetoFactoryArtifactV1 []byte

var zetoFactoryBuildV0 = solutils.MustLoadBuild(zetoFactoryArtifactV0)
var zetoFactoryBuildV1 = solutils.MustLoadBuild(zetoFactoryArtifactV1)

type zetoEventSigs struct {
	mint               string
	burn               string
	transfer           string
	transferWithEnc    string
	withdraw           string
	lock               string
	zetoLockCreated    string
	zetoLockSpent      string
	zetoLockCancelled  string
	identityRegistered string
}

type Zeto struct {
	Callbacks plugintk.DomainCallbacks

	name                 string
	config               *types.DomainFactoryConfig
	chainID              int64
	coinSchema           *prototk.StateSchema
	nftSchema            *prototk.StateSchema
	merkleTreeRootSchema *prototk.StateSchema
	merkleTreeNodeSchema *prototk.StateSchema
	dataSchema           *prototk.StateSchema
	lockInfoSchema       *prototk.StateSchema
	snarkProver          signerapi.InMemorySigner
	hasher               core.Hasher
	events               zetoEventSigs
	eventsV1             zetoEventSigs
}

type MintEvent struct {
	Outputs []pldtypes.HexUint256 `json:"outputs"`
	Data    pldtypes.HexBytes     `json:"data"`
}

type TransferEvent struct {
	Inputs  []pldtypes.HexUint256 `json:"inputs"`
	Outputs []pldtypes.HexUint256 `json:"outputs"`
	Data    pldtypes.HexBytes     `json:"data"`
}

type TransferWithEncryptedValuesEvent struct {
	Inputs          []pldtypes.HexUint256 `json:"inputs"`
	Outputs         []pldtypes.HexUint256 `json:"outputs"`
	Data            pldtypes.HexBytes     `json:"data"`
	EncryptionNonce pldtypes.HexUint256   `json:"encryptionNonce"`
	EncryptedValues []pldtypes.HexUint256 `json:"encryptedValues"`
}

type WithdrawEvent struct {
	Amount pldtypes.HexUint256   `json:"amount"`
	Inputs []pldtypes.HexUint256 `json:"inputs"`
	Output pldtypes.HexUint256   `json:"output"`
	Data   pldtypes.HexBytes     `json:"data"`
}

type LockedEvent struct {
	Inputs        []pldtypes.HexUint256 `json:"inputs"`
	Outputs       []pldtypes.HexUint256 `json:"outputs"`
	LockedOutputs []pldtypes.HexUint256 `json:"lockedOutputs"`
	Delegate      pldtypes.EthAddress   `json:"delegate"`
	Submitter     pldtypes.EthAddress   `json:"submitter"`
	Data          pldtypes.HexBytes     `json:"data"`
}

type IdentityRegisteredEvent struct {
	PublicKey []pldtypes.HexUint256 `json:"publicKey"`
	Data      pldtypes.HexBytes     `json:"data"`
}

// ZetoLockCreatedEvent matches ILockableCapability v0.5 ZetoLockCreated. ConfirmedStates include coin commitments and the lock-info row (state id = chain lockId); locked coin outputs are not written to the nullifier SMT (unlike legacy UTXOsLocked).
type ZetoLockCreatedEvent struct {
	TxId          pldtypes.Bytes32      `json:"txId"`
	LockId        pldtypes.Bytes32      `json:"lockId"`
	Owner         pldtypes.EthAddress   `json:"owner"`
	Inputs        []pldtypes.HexUint256 `json:"inputs"`
	Outputs       []pldtypes.HexUint256 `json:"outputs"`
	LockedOutputs []pldtypes.HexUint256 `json:"lockedOutputs"`
	Proof         pldtypes.HexBytes     `json:"proof"`
	Data          pldtypes.HexBytes     `json:"data"`
}

// ZetoLockSpentLikeEvent matches ZetoLockSpent and ZetoLockCancelled (same non-indexed payload shape).
type ZetoLockSpentLikeEvent struct {
	TxId          pldtypes.Bytes32      `json:"txId"`
	LockId        pldtypes.Bytes32      `json:"lockId"`
	Spender       pldtypes.EthAddress   `json:"spender"`
	LockedInputs  []pldtypes.HexUint256 `json:"lockedInputs"`
	LockedOutputs []pldtypes.HexUint256 `json:"lockedOutputs"`
	Outputs       []pldtypes.HexUint256 `json:"outputs"`
	Proof         pldtypes.HexBytes     `json:"proof"`
	Data          pldtypes.HexBytes     `json:"data"`
}

func New(callbacks plugintk.DomainCallbacks) *Zeto {
	return &Zeto{
		Callbacks: callbacks,
		hasher:    utxo.NewPoseidonHasher(),
	}
}

func (z *Zeto) Name() string {
	return z.name
}

func (z *Zeto) CoinSchemaID() string {
	return z.coinSchema.Id
}

func (z *Zeto) DataSchemaID() string {
	return z.dataSchema.Id
}

func (z *Zeto) NFTSchemaID() string {
	return z.nftSchema.Id
}
func (z *Zeto) getAlgoZetoSnarkBJJ() string {
	return zetosignerapi.AlgoDomainZetoSnarkBJJ(z.name)
}

func (z *Zeto) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
	ctx = log.WithComponent(ctx, "zeto")
	var config types.DomainFactoryConfig
	err := json.Unmarshal([]byte(req.ConfigJson), &config)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorParseDomainConfig, err)
	}

	for _, contract := range config.DomainContracts.Implementations {
		contract.Circuits.Init()
	}

	z.name = req.Name
	z.config = &config
	z.chainID = req.ChainId

	schemas, err := types.GetStateSchemas()
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorConfigZetoDomain, err)
	}

	events := getAllZetoEventAbis()
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalZetoEventAbis, err)
	}

	z.registerEventSignatures(zetoEventABISet(types.ZetoTargetContractABI_V0), &z.events)
	z.registerEventSignatures(zetoEventABISet(types.ZetoTargetContractABI_V1), &z.eventsV1)

	var signingAlgos map[string]int32
	if config.SnarkProver.CircuitsDir != "" {
		// Only build the prover and enable the algorithms for signing if circuits configured
		z.snarkProver, err = zetosigner.NewSnarkProver(&config.SnarkProver)
		if err != nil {
			return nil, err
		}
		signingAlgos = map[string]int32{
			z.getAlgoZetoSnarkBJJ(): 32,
		}
	}

	return &prototk.ConfigureDomainResponse{
		DomainConfig: &prototk.DomainConfig{
			CustomHashFunction:  true,
			AbiStateSchemasJson: schemas,
			AbiEventsJson:       string(eventsJSON),
			SigningAlgorithms:   signingAlgos,
		},
	}, nil
}

func (z *Zeto) InitDomain(ctx context.Context, req *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
	z.coinSchema = req.AbiStateSchemas[0]
	z.nftSchema = req.AbiStateSchemas[1]
	z.merkleTreeRootSchema = req.AbiStateSchemas[2]
	z.merkleTreeNodeSchema = req.AbiStateSchemas[3]
	z.dataSchema = req.AbiStateSchemas[4]
	z.lockInfoSchema = nil
	if len(req.AbiStateSchemas) > 5 {
		z.lockInfoSchema = req.AbiStateSchemas[5]
	}

	return &prototk.InitDomainResponse{}, nil
}

func (z *Zeto) InitDeploy(ctx context.Context, req *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
	ctx, _, err := z.validateDeployAndGetLogContext(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateInitDeployParams, err)
	}
	return &prototk.InitDeployResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       req.Transaction.From,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}, nil
}

func (z *Zeto) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
	ctx, initParams, err := z.validateDeployAndGetLogContext(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidatePrepDeployParams, err)
	}
	useV1 := strings.EqualFold(initParams.DomainConfigSchema, types.DomainConfigSchemaV1)
	if !useV1 && (initParams.CircuitBundleId != "" || initParams.ZetoVariant != 0) {
		return nil, i18n.NewError(ctx, msgs.MsgZetoV1SchemaRequired)
	}

	effectiveFV := z.config.FactoryVersion
	if initParams.FactoryVersion != 0 {
		if z.config.FactoryVersion != 0 && initParams.FactoryVersion != z.config.FactoryVersion {
			return nil, i18n.NewError(ctx, msgs.MsgZetoFactoryVersionConflict, initParams.FactoryVersion, z.config.FactoryVersion)
		}
		effectiveFV = initParams.FactoryVersion
	}
	var factoryABI abi.ABI
	switch effectiveFV {
	case int64(types.ZetoPaladinFactoryV0):
		factoryABI = zetoFactoryBuildV0.ABI
	case int64(types.ZetoPaladinFactoryV1):
		factoryABI = zetoFactoryBuildV1.ABI
	default:
		return nil, i18n.NewError(ctx, msgs.MsgUnsupportedZetoFactoryVersion, effectiveFV)
	}
	deployFn, err := pickZetoFactoryDeploy7Arg(factoryABI)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidatePrepDeployParams, err.Error())
	}

	circuits, err := z.config.GetCircuitsForDeploy(ctx, initParams.TokenName, initParams.ZetoVariant, initParams.CircuitBundleId)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindCircuitId, err)
	}

	config := &types.DomainInstanceConfig{
		Circuits:        circuits,
		TokenName:       initParams.TokenName,
		ZetoVariant:     pldtypes.HexUint64(initParams.ZetoVariant),
		FactoryVersion:  effectiveFV,
		CircuitBundleId: initParams.CircuitBundleId,
	}

	var encoded pldtypes.HexBytes
	if useV1 {
		config.ConfigSchema = types.DomainConfigSchemaV1
		encoded, err = types.EncodeDomainInstanceConfigV1(ctx, config)
	} else {
		config.ConfigSchema = types.DomainConfigSchemaV0
		encoded, err = types.EncodeDomainInstanceConfigV0(ctx, config)
	}
	if err != nil {
		return nil, err
	}

	deployParams := &types.DeployParams{
		TransactionID: req.Transaction.TransactionId,
		Data:          pldtypes.HexBytes(encoded),
		TokenName:     initParams.TokenName,
		Name:          initParams.TokenName,
		Symbol:        initParams.TokenName,
		InitialOwner:  req.ResolvedVerifiers[0].Verifier, // TODO: allow the initial owner to be specified by the deploy request
		IsNonFungible: common.IsNonFungibleToken(initParams.TokenName),
	}
	paramsJSON, err := json.Marshal(deployParams)
	if err != nil {
		return nil, err
	}
	functionJSON, err := json.Marshal(deployFn)
	if err != nil {
		return nil, err
	}

	from := req.Transaction.From
	return &prototk.PrepareDeployResponse{
		Transaction: &prototk.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
		Signer: &from,
	}, nil
}

func (z *Zeto) InitContract(ctx context.Context, req *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
	ctx = log.WithComponent(ctx, "zeto")
	var zetoContractConfigJSON []byte
	domainConfig, err := z.decodeDomainConfig(ctx, req.ContractConfig)
	if err == nil {
		zetoContractConfigJSON, err = json.Marshal(domainConfig)
	}
	if err != nil {
		// This on-chain contract has invalid configuration - not an error in our process
		return &prototk.InitContractResponse{Valid: false}, nil
	}

	return &prototk.InitContractResponse{
		Valid: true,
		ContractConfig: &prototk.ContractConfig{
			ContractConfigJson:   string(zetoContractConfigJSON),
			CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
			SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
		},
	}, nil
}

func (z *Zeto) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	ctx, tx, handler, err := z.validateTransactionAndGetLogContext(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateInitTxSpec, err)
	}
	return handler.Init(ctx, tx, req)
}

func (z *Zeto) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	ctx, tx, handler, err := z.validateTransactionAndGetLogContext(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateAssembleTxSpec, err)
	}
	return handler.Assemble(ctx, tx, req)
}

func (z *Zeto) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	ctx, tx, handler, err := z.validateTransactionAndGetLogContext(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateEndorseTxParams, err)
	}
	return handler.Endorse(ctx, tx, req)
}

func (z *Zeto) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	ctx, tx, handler, err := z.validateTransactionAndGetLogContext(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidatePrepTxSpec, err)
	}
	return handler.Prepare(ctx, tx, req)
}

func (z *Zeto) GetHandler(method, tokenName string, zetoVariant pldtypes.HexUint64) types.DomainHandler {
	fungibleV0 := zetoVariant == types.ZetoFungibleV0ABI
	if common.IsNonFungibleToken(tokenName) {
		switch method {
		case types.METHOD_MINT:
			return nonfungible.NewMintHandler(z.name, z.nftSchema)
		case types.METHOD_TRANSFER:
			return nonfungible.NewTransferHandler(z.name, z.Callbacks, z.nftSchema, z.merkleTreeRootSchema, z.merkleTreeNodeSchema)
		default:
			return nil
		}
	}
	switch method {
	case types.METHOD_MINT:
		return fungible.NewMintHandler(z.name, z.coinSchema, z.dataSchema)
	case types.METHOD_TRANSFER:
		return fungible.NewTransferHandler(z.name, z.Callbacks, z.coinSchema, z.merkleTreeRootSchema, z.merkleTreeNodeSchema, z.dataSchema)
	case types.METHOD_TRANSFER_LOCKED:
		if !fungibleV0 {
			return nil
		}
		return fungible.NewTransferLockedHandler(z.name, z.Callbacks, z.coinSchema, z.merkleTreeRootSchema, z.merkleTreeNodeSchema, z.dataSchema)
	case types.METHOD_LOCK:
		if !fungibleV0 {
			return nil
		}
		return fungible.NewLockHandler(z.name, z.Callbacks, z.coinSchema, z.merkleTreeRootSchema, z.merkleTreeNodeSchema)
	case types.METHOD_CREATE_LOCK:
		if fungibleV0 {
			return nil
		}
		return fungible.NewCreateLockHandler(z.name, z.Callbacks, z.coinSchema, z.merkleTreeRootSchema, z.merkleTreeNodeSchema, z.lockInfoSchema)
	case types.METHOD_SPEND_LOCK:
		if fungibleV0 {
			return nil
		}
		return fungible.NewSpendLockHandler(z.name, z.Callbacks, z.coinSchema, z.merkleTreeRootSchema, z.merkleTreeNodeSchema, z.dataSchema, z.lockInfoSchema)
	case types.METHOD_DEPOSIT:
		return fungible.NewDepositHandler(z.name, z.coinSchema)
	case types.METHOD_WITHDRAW:
		return fungible.NewWithdrawHandler(z.name, z.Callbacks, z.coinSchema, z.merkleTreeRootSchema, z.merkleTreeNodeSchema)
	default:
		return nil
	}
}

func (z *Zeto) GetCallHandler(method, tokenName string) types.DomainCallHandler {
	switch method {
	case types.METHOD_BALANCE_OF:
		return fungible.NewBalanceOfHandler(z.name, z.Callbacks, z.coinSchema)
	default:
		return nil
	}
}

func (z *Zeto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*types.DomainInstanceConfig, error) {
	return types.DecodeDomainInstanceConfig(ctx, domainConfig)
}

func (z *Zeto) validateDeploy(tx *prototk.DeployTransactionSpecification) (*types.InitializerParams, error) {
	var params types.InitializerParams
	err := json.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (z *Zeto) validateDeployAndGetLogContext(ctx context.Context, txSpec *prototk.DeployTransactionSpecification) (context.Context, *types.InitializerParams, error) {
	ctx = log.WithComponent(ctx, "zeto")
	ctx = log.WithLogField(ctx, "tx", txSpec.TransactionId)

	params, err := z.validateDeploy(txSpec)
	if err != nil {
		return ctx, nil, err
	}
	return ctx, params, nil
}

func validateTransactionCommon[T any](
	ctx context.Context,
	tx *prototk.TransactionSpecification,
	getHandler func(method, tokenName string, zetoVariant pldtypes.HexUint64) T,
) (*types.ParsedTransaction, T, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		var zero T
		return nil, zero, i18n.NewError(ctx, msgs.MsgErrorUnmarshalFuncAbi, err)
	}

	var domainConfig *types.DomainInstanceConfig
	err = json.Unmarshal([]byte(tx.ContractInfo.ContractConfigJson), &domainConfig)
	if err != nil {
		var zero T
		return nil, zero, err
	}

	var fnEntry *abi.Entry
	if common.IsNonFungibleToken(domainConfig.TokenName) {
		fnEntry = types.ZetoNonFungibleABI.Functions()[functionABI.Name]
	} else {
		fnEntry = types.ZetoFungibleFunctionForVariant(domainConfig.ZetoVariant, functionABI.Name)
	}

	handler := getHandler(functionABI.Name, domainConfig.TokenName, domainConfig.ZetoVariant)
	handlerValue := reflect.ValueOf(handler)
	if fnEntry == nil || handlerValue.IsNil() {
		var zero T
		return nil, zero, i18n.NewError(ctx, msgs.MsgUnknownFunction, functionABI.Name)
	}

	validator, ok := any(handler).(interface {
		ValidateParams(ctx context.Context, domainConfig *types.DomainInstanceConfig, paramsJson string) (any, error)
	})
	if !ok {
		var zero T
		return nil, zero, i18n.NewError(ctx, msgs.MsgErrorHandlerImplementationNotFound)
	}

	params, err := validator.ValidateParams(ctx, domainConfig, tx.FunctionParamsJson)
	if err != nil {
		var zero T
		return nil, zero, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, err)
	}

	signature := fnEntry.SolString()
	if tx.FunctionSignature != signature {
		var zero T
		return nil, zero, i18n.NewError(ctx, msgs.MsgUnexpectedFuncSignature, functionABI.Name, signature, tx.FunctionSignature)
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractInfo.ContractAddress)
	if err != nil {
		var zero T
		return nil, zero, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}

	return &types.ParsedTransaction{
		Transaction:     tx,
		FunctionABI:     &functionABI,
		ContractAddress: contractAddress,
		DomainConfig:    domainConfig,
		Params:          params,
	}, handler, nil
}

func (z *Zeto) validateTransaction(ctx context.Context, tx *prototk.TransactionSpecification) (*types.ParsedTransaction, types.DomainHandler, error) {
	return validateTransactionCommon(
		ctx,
		tx,
		z.GetHandler,
	)
}

func (z *Zeto) validateTransactionAndGetLogContext(ctx context.Context, txSpec *prototk.TransactionSpecification) (context.Context, *types.ParsedTransaction, types.DomainHandler, error) {
	ctx = log.WithComponent(ctx, "zeto")
	tx, handler, err := z.validateTransaction(ctx, txSpec)
	if err != nil {
		return ctx, nil, nil, err
	}

	ctx = log.WithLogField(ctx, "tx", tx.Transaction.TransactionId)
	ctx = log.WithLogField(ctx, "contract", tx.Transaction.ContractInfo.ContractAddress)
	return ctx, tx, handler, nil
}

func (z *Zeto) validateCallAndGetLogContext(ctx context.Context, callSpec *prototk.TransactionSpecification) (context.Context, *types.ParsedTransaction, types.DomainCallHandler, error) {
	ctx = log.WithComponent(ctx, "zeto")
	call, handler, err := z.validateCall(ctx, callSpec)
	if err != nil {
		return ctx, nil, nil, err
	}

	ctx = log.WithLogField(ctx, "tx", call.Transaction.TransactionId)
	ctx = log.WithLogField(ctx, "contract", call.Transaction.ContractInfo.ContractAddress)
	return ctx, call, handler, nil
}

func (z *Zeto) validateCall(ctx context.Context, call *prototk.TransactionSpecification) (*types.ParsedTransaction, types.DomainCallHandler, error) {
	return validateTransactionCommon(
		ctx,
		call,
		func(m, tn string, _ pldtypes.HexUint64) types.DomainCallHandler {
			return z.GetCallHandler(m, tn)
		},
	)
}

func (z *Zeto) registerEventSignatures(source abi.ABI, dest *zetoEventSigs) {
	for _, event := range source {
		if event.Type != abi.Event {
			continue
		}
		switch event.Name {
		case "UTXOMint":
			dest.mint = event.SolString()
		case "UTXOBurn":
			dest.burn = event.SolString()
		case "UTXOTransfer":
			dest.transfer = event.SolString()
		case "UTXOTransferWithEncryptedValues":
			dest.transferWithEnc = event.SolString()
		case "UTXOWithdraw":
			dest.withdraw = event.SolString()
		case "UTXOsLocked":
			dest.lock = event.SolString()
		case "ZetoLockCreated":
			dest.zetoLockCreated = event.SolString()
		case "ZetoLockSpent":
			dest.zetoLockSpent = event.SolString()
		case "ZetoLockCancelled":
			dest.zetoLockCancelled = event.SolString()
		case "IdentityRegistered":
			dest.identityRegistered = event.SolString()
		}
	}
}

func (z *Zeto) HandleEventBatch(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
	ctx = log.WithComponent(ctx, "zeto")
	ctx = log.WithLogField(ctx, "contract", req.ContractInfo.ContractAddress)

	var domainConfig *types.DomainInstanceConfig
	err := json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorAbiDecodeDomainInstanceConfig, err)
	}

	contractAddress, err := pldtypes.ParseEthAddress(req.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}

	var res prototk.HandleEventBatchResponse
	var errors []string
	var smtForStates *common.MerkleTreeSpec
	var smtForLockedStates *common.MerkleTreeSpec
	var smtForKyc *common.MerkleTreeSpec
	if common.IsNullifiersToken(domainConfig.TokenName) {
		smtName := zetosmt.MerkleTreeName(domainConfig.TokenName, contractAddress)
		smtForStates, err = common.NewMerkleTreeSpec(ctx, smtName, smt.StatesTree, z.Callbacks, z.merkleTreeRootSchema.Id, z.merkleTreeNodeSchema.Id, req.StateQueryContext)
		if err != nil {
			return nil, err
		}
		smtName = zetosmt.MerkleTreeNameForLockedStates(domainConfig.TokenName, contractAddress)
		smtForLockedStates, err = common.NewMerkleTreeSpec(ctx, smtName, smt.LockedStatesTree, z.Callbacks, z.merkleTreeRootSchema.Id, z.merkleTreeNodeSchema.Id, req.StateQueryContext)
		if err != nil {
			return nil, err
		}
	}
	if common.IsKycToken(domainConfig.TokenName) {
		smtName := zetosmt.MerkleTreeNameForKycStates(domainConfig.TokenName, contractAddress)
		smtForKyc, err = common.NewMerkleTreeSpec(ctx, smtName, smt.KycStatesTree, z.Callbacks, z.merkleTreeRootSchema.Id, z.merkleTreeNodeSchema.Id, req.StateQueryContext)
		if err != nil {
			return nil, err
		}
	}
	for _, ev := range req.Events {
		var err error
		switch ev.SoliditySignature {
		case z.events.mint, z.eventsV1.mint:
			err = z.handleMintEvent(ctx, smtForStates, ev, domainConfig.TokenName, &res)
		case z.events.transfer, z.eventsV1.transfer:
			err = z.handleTransferEvent(ctx, smtForStates, ev, domainConfig.TokenName, &res)
		case z.events.transferWithEnc, z.eventsV1.transferWithEnc:
			err = z.handleTransferWithEncryptionEvent(ctx, smtForStates, ev, domainConfig.TokenName, &res)
		case z.events.withdraw, z.eventsV1.withdraw:
			err = z.handleWithdrawEvent(ctx, smtForStates, ev, domainConfig.TokenName, &res)
		case z.events.lock, z.eventsV1.lock:
			err = z.handleLockedEvent(ctx, smtForStates, smtForLockedStates, ev, domainConfig.TokenName, &res)
		case z.eventsV1.zetoLockCreated:
			err = z.handleZetoLockCreatedEvent(ctx, req.StateQueryContext, smtForStates, ev, domainConfig.TokenName, &res)
		case z.eventsV1.zetoLockSpent:
			err = z.handleZetoLockSpentLikeEvent(ctx, req.StateQueryContext, smtForStates, ev, domainConfig.TokenName, &res, "ZetoLockSpent")
		case z.eventsV1.zetoLockCancelled:
			err = z.handleZetoLockSpentLikeEvent(ctx, req.StateQueryContext, smtForStates, ev, domainConfig.TokenName, &res, "ZetoLockCancelled")
		case z.events.identityRegistered, z.eventsV1.identityRegistered:
			err = z.handleIdentityRegisteredEvent(ctx, smtForKyc, ev, domainConfig.TokenName, &res)
		}
		if err != nil {
			errors = append(errors, err.Error())
		}
		if err == nil && common.IsNullifiersToken(domainConfig.TokenName) {
			newStatesForSMT, smtErr := smtForStates.Storage.GetNewStates(ctx)
			if smtErr != nil {
				return nil, i18n.NewError(ctx, pldmsgs.MsgErrorGetNewSmtStates, smtForStates.Name, smtErr)
			}
			if len(newStatesForSMT) > 0 {
				res.NewStates = append(res.NewStates, newStatesForSMT...)
			}
			newStatesForSMTForLocked, smtErr := smtForLockedStates.Storage.GetNewStates(ctx)
			if smtErr != nil {
				return nil, i18n.NewError(ctx, pldmsgs.MsgErrorGetNewSmtStates, smtForLockedStates.Name, smtErr)
			}
			if len(newStatesForSMTForLocked) > 0 {
				res.NewStates = append(res.NewStates, newStatesForSMTForLocked...)
			}
			if common.IsKycToken(domainConfig.TokenName) {
				newStatesForSMTForKyc, smtErr := smtForKyc.Storage.GetNewStates(ctx)
				log.L(ctx).Debugf("New KYC states from SMT %s: %+v", smtForKyc.Name, newStatesForSMTForKyc)
				if smtErr != nil {
					return nil, i18n.NewError(ctx, pldmsgs.MsgErrorGetNewSmtStates, smtForKyc.Name, smtErr)
				}
				if len(newStatesForSMTForKyc) > 0 {
					res.NewStates = append(res.NewStates, newStatesForSMTForKyc...)
				}
			}
		}
	}
	if len(errors) > 0 {
		return &res, i18n.NewError(ctx, msgs.MsgErrorHandleEvents, formatErrors(errors))
	}
	return &res, nil
}

func (z *Zeto) GetVerifier(ctx context.Context, req *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
	ctx = log.WithComponent(ctx, "zeto")
	verifier, err := z.snarkProver.GetVerifier(ctx, req.Algorithm, req.VerifierType, req.PrivateKey)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorGetVerifier, err)
	}
	return &prototk.GetVerifierResponse{
		Verifier: verifier,
	}, nil
}

func (z *Zeto) Sign(ctx context.Context, req *prototk.SignRequest) (*prototk.SignResponse, error) {
	ctx = log.WithComponent(ctx, "zeto")
	switch req.PayloadType {
	case zetosignerapi.PAYLOAD_DOMAIN_ZETO_NULLIFIER:
		var coin *types.ZetoCoin
		var hashInt *big.Int
		keyPair, err := signercommon.NewBabyJubJubPrivateKey(req.PrivateKey)
		if err == nil {
			err = json.Unmarshal(req.Payload, &coin)
		}
		if err == nil {
			hashInt, err = signercommon.CalculateNullifier(coin.Amount.Int(), coin.Salt.Int(), babyjub.SkToBigInt(keyPair))
		}
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgNullifierGenerationFailed)
		}
		return &prototk.SignResponse{
			Payload: common.IntTo32ByteSlice(hashInt),
		}, nil
	case zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK:
		proof, err := z.snarkProver.Sign(ctx, req.Algorithm, req.PayloadType, req.PrivateKey, req.Payload)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorSign, err)
		}
		return &prototk.SignResponse{
			Payload: proof,
		}, nil
	default:
		return nil, i18n.NewError(ctx, msgs.MsgUnknownSignPayload, req.PayloadType)
	}
}

func (z *Zeto) validateCoinState(ctx context.Context, state *prototk.EndorsableState) (string, error) {
	log.L(ctx).Debugf("validating coin state hash: %+v\n", state)
	var coin types.ZetoCoin
	err := json.Unmarshal([]byte(state.StateDataJson), &coin)
	if err != nil {
		log.L(ctx).Errorf("Error unmarshalling coin state data: %s", err)
		return "", i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
	}
	hash, err := coin.Hash(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error hashing coin state data: %s", err)
		return "", i18n.NewError(ctx, msgs.MsgErrorHashOutputState, err)
	}
	return z.validateStateHash(ctx, hash, state)
}

func (z *Zeto) validateDataState(ctx context.Context, state *prototk.EndorsableState) (string, error) {
	log.L(ctx).Debugf("validating data state hash: %+v\n", state)
	var info types.TransactionData
	err := json.Unmarshal([]byte(state.StateDataJson), &info)
	if err != nil {
		log.L(ctx).Errorf("Error unmarshalling data state data: %s", err)
		return "", i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
	}
	hash, err := info.Hash(ctx)
	if err != nil {
		log.L(ctx).Errorf("Error hashing data state data: %s", err)
		return "", i18n.NewError(ctx, msgs.MsgErrorHashOutputState, err)
	}
	return z.validateStateHash(ctx, hash, state)
}

func (z *Zeto) validateStateHash(ctx context.Context, hash *pldtypes.HexUint256, state *prototk.EndorsableState) (string, error) {
	hashString := common.HexUint256To32ByteHexString(hash)
	if state.Id == "" {
		// if the requested state ID is empty, we simply set it
		return hashString, nil
	}
	// if the requested state ID is set, we compare it with the calculated hash
	stateId, _ := pldtypes.ParseHexUint256(ctx, state.Id)
	if stateId == nil || hash.Int().Cmp(stateId.Int()) != 0 {
		log.L(ctx).Errorf("State hash mismatch (hashed vs. received): %s != %s", hash.String(), state.Id)
		return "", i18n.NewError(ctx, msgs.MsgErrorStateHashMismatch, hash.String(), state.Id)
	}
	return state.Id, nil
}

func (z *Zeto) ValidateStateHashes(ctx context.Context, req *prototk.ValidateStateHashesRequest) (_ *prototk.ValidateStateHashesResponse, err error) {
	ctx = log.WithComponent(ctx, "zeto")
	var res prototk.ValidateStateHashesResponse
	for _, state := range req.States {
		var id string
		switch state.SchemaId {
		case z.CoinSchemaID():
			if id, err = z.validateCoinState(ctx, state); err != nil {
				return nil, err
			}
		case z.DataSchemaID():
			if id, err = z.validateDataState(ctx, state); err != nil {
				return nil, err
			}
		}
		res.StateIds = append(res.StateIds, id)

	}
	return &res, nil
}

func (z *Zeto) InitCall(ctx context.Context, req *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
	ctx, ptx, handler, err := z.validateCallAndGetLogContext(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateInitCallTxSpec, err)
	}
	return handler.InitCall(ctx, ptx, req)
}

func (z *Zeto) ExecCall(ctx context.Context, req *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
	ctx, ptx, handler, err := z.validateCallAndGetLogContext(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateExecCallTxSpec, err)
	}
	return handler.ExecCall(ctx, ptx, req)
}

func (z *Zeto) BuildReceipt(ctx context.Context, req *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error) {
	// TODO: Event logs for transfers would be great for Noto
	return nil, i18n.NewError(ctx, msgs.MsgNoDomainReceipt)
}

func (z *Zeto) ConfigurePrivacyGroup(ctx context.Context, req *prototk.ConfigurePrivacyGroupRequest) (*prototk.ConfigurePrivacyGroupResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (z *Zeto) InitPrivacyGroup(ctx context.Context, req *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (z *Zeto) WrapPrivacyGroupEVMTX(ctx context.Context, req *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (z *Zeto) CheckStateCompletion(ctx context.Context, req *prototk.CheckStateCompletionRequest) (*prototk.CheckStateCompletionResponse, error) {
	return &prototk.CheckStateCompletionResponse{
		// TODO: Implement manifests similar to noto, to allow receipts to be delivered to listeners reliably with partial state avaialability.
		NextMissingStateId: req.UnavailableStates.FirstUnavailableId,
	}, nil
}

func (z *Zeto) IsBaseLedgerRevertRetryable(_ context.Context, _ *prototk.IsBaseLedgerRevertRetryableRequest) (*prototk.IsBaseLedgerRevertRetryableResponse, error) {
	// TODO: this is just defaulting to the sequencer retry threshold and could be customized by the domain
	return &prototk.IsBaseLedgerRevertRetryableResponse{Retryable: true}, nil
}

func (z *Zeto) InvokeRPC(ctx context.Context, _ *prototk.InvokeRPCRequest) (*prototk.InvokeRPCResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}
