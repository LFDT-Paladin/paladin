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

package domainmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/query"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type PrivateSmartContract struct {
	DeployTX        uuid.UUID           `json:"deployTransaction"   gorm:"column:deploy_tx"`
	RegistryAddress pldtypes.EthAddress `json:"domainAddress"       gorm:"column:domain_address"`
	Address         pldtypes.EthAddress `json:"address"             gorm:"column:address"`
	ConfigBytes     pldtypes.HexBytes   `json:"configBytes"         gorm:"column:config_bytes"`
	Created         pldtypes.Timestamp  `json:"created"             gorm:"column:created;autoCreateTime:nano"`
}

type domainContract struct {
	dm     *domainManager
	d      *domain
	api    components.DomainManagerToDomain
	info   *PrivateSmartContract   // from the DB
	config *prototk.ContractConfig // from init processing in the domain
}

type pscLoadResult int

const (
	pscLoadError pscLoadResult = iota
	pscInitError
	pscNotFound
	pscDomainNotFound
	pscInvalid
	pscValid
)

func (d *domain) initSmartContract(ctx context.Context, dbTX persistence.DBTX, def *PrivateSmartContract) (pscLoadResult, *domainContract, error) {
	dc := &domainContract{
		dm:   d.dm,
		d:    d,
		api:  d.api,
		info: def,
	}
	var privacyGroup *prototk.PrivacyGroup
	pg, err := d.dm.groupManager.QueryGroups(ctx, dbTX, query.NewQueryBuilder().Equal("domain", d.name).Equal("contractAddress", &def.Address).Limit(1).Query())
	if err != nil {
		return pscInitError, nil, err
	}
	if len(pg) > 0 {
		privacyGroup = mapPrivacyGroupToProto(pg[0].ID, pg[0].GenesisStateData())
	}

	res, err := d.api.InitContract(ctx, &prototk.InitContractRequest{
		ContractAddress: def.Address.String(),
		ContractConfig:  def.ConfigBytes,
		PrivacyGroup:    privacyGroup,
	})
	if err != nil {
		log.L(ctx).Errorf("Error initializing smart contract address: %s with config %s :  %s", def.Address, def.ConfigBytes.HexString(), err.Error())
		return pscInitError, nil, err
	}
	if !res.Valid {
		log.L(ctx).Warnf("smart contract %s has invalid configuration rejected by the domain", def.Address)
		return pscInvalid, nil, nil
	}
	dc.config = res.ContractConfig

	// Only cache valid ones
	d.dm.contractCache.Set(dc.info.Address, dc)
	return pscValid, dc, nil
}

func (dc *domainContract) buildTransactionSpecification(ctx context.Context, localTx *components.ResolvedTransaction, intent prototk.TransactionSpecification_Intent) (*prototk.TransactionSpecification, error) {

	if localTx.Transaction == nil || localTx.Transaction.Data == nil || localTx.Function == nil ||
		localTx.Transaction.Domain != dc.Domain().Name() || *localTx.Transaction.To != dc.info.Address {
		log.L(ctx).Errorf("Invalid tx for domain %s/%s: %+v", dc.Domain().Name(), dc.info.Address, localTx.Transaction)
		return nil, i18n.NewError(ctx, msgs.MsgDomainTxnInputDefinitionInvalid)
	}

	latestConfirmedBlock, err := dc.dm.blockIndexer.GetLatestConfirmedBlockMetadata(ctx)
	if err != nil {
		return nil, err
	}

	var abiJSON []byte
	fnDef := localTx.Function.Definition
	inputValues, err := fnDef.Inputs.ParseJSONCtx(ctx, localTx.Transaction.Data)
	if err == nil {
		abiJSON, err = json.Marshal(fnDef)
	}
	var paramsJSON []byte
	if err == nil {
		// Serialize to standardized JSON before passing to domain
		paramsJSON, err = pldtypes.StandardABISerializer().SerializeJSONCtx(ctx, inputValues)
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidFunctionParams, fnDef.SolString())
	}

	return &prototk.TransactionSpecification{
		ContractInfo: &prototk.ContractInfo{
			ContractAddress:    dc.info.Address.String(),
			ContractConfigJson: dc.config.ContractConfigJson,
		},
		From:               localTx.Transaction.From,
		FunctionAbiJson:    string(abiJSON),
		FunctionParamsJson: string(paramsJSON),
		FunctionSignature:  fnDef.SolString(), // we use the proprietary "Solidity inspired" form that is very specific, including param names and nested struct defs
		BaseBlock:          latestConfirmedBlock.Number,
		Intent:             intent,
		BaseBlockTimestamp: latestConfirmedBlock.Timestamp,
	}, nil
}

func (dc *domainContract) ContractConfig() *prototk.ContractConfig {
	return dc.config
}

func (dc *domainContract) InitTransaction(ctx context.Context, tx *components.PrivateTransaction, localTx *components.ResolvedTransaction) error {

	txSpec, err := dc.buildTransactionSpecification(ctx, localTx, tx.Intent)
	if err != nil {
		return err
	}
	txSpec.TransactionId = pldtypes.Bytes32UUIDFirst16(tx.ID).String()
	tx.ID = *localTx.Transaction.ID
	tx.Domain = localTx.Transaction.Domain
	tx.Address = *localTx.Transaction.To

	// Do the request with the domain
	log.L(ctx).Infof("Initializing transaction=%s domain=%s contract-address=%s", tx.ID, dc.d.name, txSpec.ContractInfo.ContractAddress)
	res, err := dc.api.InitTransaction(ctx, &prototk.InitTransactionRequest{
		Transaction: txSpec,
	})
	if err != nil {
		return err
	}

	// Store the response back on the TX
	preAssembly := &components.TransactionPreAssembly{
		TransactionSpecification: txSpec,
		RequiredVerifiers:        res.RequiredVerifiers,
		PublicTxOptions:          localTx.Transaction.PublicTxOptions,
	}
	tx.PreAssembly = preAssembly
	return nil
}

func setLocalNode(localNode, lookup string) string {
	if strings.Contains(lookup, "@") {
		return lookup
	}
	return fmt.Sprintf("%s@%s", lookup, localNode)
}

func (dc *domainContract) fullyQualifyAssemblyIdentities(res *prototk.AssembleTransactionResponse) {
	localNode := dc.dm.transportMgr.LocalNodeName()
	for _, ap := range res.AttestationPlan {
		for i := range ap.Parties {
			ap.Parties[i] = setLocalNode(localNode, ap.Parties[i])
		}
	}
	for _, sp := range res.AssembledTransaction.OutputStates {
		for i := range sp.DistributionList {
			sp.DistributionList[i] = setLocalNode(localNode, sp.DistributionList[i])
		}
		for i := range sp.NullifierSpecs {
			sp.NullifierSpecs[i].Party = setLocalNode(localNode, sp.NullifierSpecs[i].Party)
		}
	}
	for _, sp := range res.AssembledTransaction.InfoStates {
		for i := range sp.DistributionList {
			sp.DistributionList[i] = setLocalNode(localNode, sp.DistributionList[i])
		}
		for i := range sp.NullifierSpecs {
			sp.NullifierSpecs[i].Party = setLocalNode(localNode, sp.NullifierSpecs[i].Party)
		}
	}
}

func (dc *domainContract) AssembleTransaction(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, tx *components.PrivateTransaction, localTx *components.ResolvedTransaction, resolvedVerifiers []*prototk.ResolvedVerifier) error {
	if tx.PreAssembly == nil || localTx.Transaction == nil || localTx.Transaction.ID == nil || *localTx.Transaction.ID != tx.ID {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompleteAssembleTransaction)
	}

	// Assemble is a sender-role operation, that must be performed only using the local details of the transaction as submitted
	// to this node by the application connected to that node. We cannot use any data that was received over the wire
	// from the coordinator as part of the assembly.
	txSpec, err := dc.buildTransactionSpecification(ctx, localTx, tx.Intent)
	if err != nil {
		return err
	}
	txSpec.TransactionId = pldtypes.Bytes32UUIDFirst16(tx.ID).String()
	tx.PreAssembly.TransactionSpecification = txSpec

	// Clear any previous assembly state out, as it's considered completely invalid
	// at this point if we're re-assembling.
	preAssembly := tx.PreAssembly

	c := dc.d.newInFlightDomainRequest(readTX, dqc, true)
	defer c.close()

	// Now we have the required verifiers, we can ask the domain to do the heavy lifting
	// and assemble the transaction (using the state store interface we provide)
	log.L(ctx).Info("Assembling transaction")
	res, err := dc.api.AssembleTransaction(ctx, &prototk.AssembleTransactionRequest{
		StateQueryContext: c.id,
		Transaction:       preAssembly.TransactionSpecification,
		ResolvedVerifiers: resolvedVerifiers,
	})
	if err != nil {
		return err
	}

	postAssembly := &components.TransactionPostAssembly{}
	// If the result is not OK (e.g. there is a REVERT) then we return the situation to the private TX manager to handle
	if res.AssemblyResult == prototk.AssembleTransactionResponse_OK && res.AssembledTransaction != nil {
		// We hydrate the states on our side of the Manager<->Plugin divide at this point,
		// which provides back to the engine the full sequence locking information of the
		// states (inputs, and read)
		log.L(ctx).Debugf("Loading post-assembly input states from context")
		postAssembly.InputStates, err = dc.loadStatesFromContext(ctx, dqc, readTX, res.AssembledTransaction.InputStates)
		if err != nil {
			return err
		}
		log.L(ctx).Debugf("Loading post-assembly read states from context")
		postAssembly.ReadStates, err = dc.loadStatesFromContext(ctx, dqc, readTX, res.AssembledTransaction.ReadStates)
		if err != nil {
			return err
		}

		// At this point we resolve all verifiers to their fully qualified variants - as the local node needs to be the assembling node.
		// - Attestation plan identities
		// - State distributions
		// - Associated nullifier requests
		log.L(ctx).Info("Qualifying assembly identities")
		dc.fullyQualifyAssemblyIdentities(res)

		// Note the states at this point are just potential states - depending on the analysis
		// of the result, and the locking on the input states, the engine might decide to
		// abandon this attempt and just re-assemble later.
		log.L(ctx).Debugf("Number of potential output states: %+v", len(res.AssembledTransaction.OutputStates))
		log.L(ctx).Debugf("Number of potential info states: %+v", len(res.AssembledTransaction.InfoStates))
		postAssembly.OutputStatesPotential = res.AssembledTransaction.OutputStates
		postAssembly.InfoStatesPotential = res.AssembledTransaction.InfoStates
		postAssembly.DomainData = res.AssembledTransaction.DomainData
	}

	// We need to pass the assembly result back - it needs to be assigned to a sequence
	// before anything interesting can happen with the result here
	postAssembly.RevertReason = res.RevertReason
	postAssembly.AssemblyResult = res.AssemblyResult
	postAssembly.AttestationPlan = res.AttestationPlan
	tx.PostAssembly = postAssembly
	return nil
}

// Happens only on the sequencing node
func (dc *domainContract) WritePotentialStates(ctx context.Context, dsw components.DomainStateWriter, readTX persistence.DBTX, tx *components.PrivateTransaction) (err error) {
	if tx.PreAssembly == nil || tx.PreAssembly.TransactionSpecification == nil || tx.PostAssembly == nil {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompleteWritePotentialStates)
	}

	// Now we're confident enough about this transaction to (on the sequencer) to have allocated
	// it to a sequence, and we want to write the OutputStatesPotential+InfoStatesPotential arrays:
	// 1) Writing them to the DB (unflushed at this point)
	// 2) Storing their identifiers into the OutputStatesFull list
	//
	// Note: This only happens on the sequencer node - any endorsing nodes just take the Full states
	//       and write them directly to the sequence prior to endorsement
	postAssembly := tx.PostAssembly
	log.L(ctx).Debugf("WritePotentialStates: Writing %+v output states", len(postAssembly.OutputStatesPotential))
	postAssembly.OutputStates, err = dc.upsertPotentialStates(ctx, dsw, readTX, tx, postAssembly.OutputStatesPotential, true)
	if err == nil {
		log.L(ctx).Debugf("WritePotentialStates: Writing %+v info potentialstates", len(postAssembly.InfoStatesPotential))
		postAssembly.InfoStates, err = dc.upsertPotentialStates(ctx, dsw, readTX, tx, postAssembly.InfoStatesPotential, false)
	}
	return err
}

func (dc *domainContract) MapPotentialStates(ctx context.Context, potentialStates []*prototk.NewState, outputStates bool, createdByTX *components.PrivateTransaction) (stateUpserts []*components.StateUpsert, err error) {
	return dc.d.mapPotentialStates(ctx, potentialStates, outputStates, createdByTX)
}

func (dc *domainContract) upsertPotentialStates(ctx context.Context, dsw components.DomainStateWriter, readTX persistence.DBTX, tx *components.PrivateTransaction, potentialStates []*prototk.NewState, isOutput bool) (writtenStates []*components.FullState, err error) {
	newStatesToWrite, err := dc.d.mapPotentialStates(ctx, potentialStates, isOutput, tx)
	if err != nil {
		return nil, err
	}
	log.L(ctx).Debugf("upsertPotentialStates: %d states to write", len(potentialStates))

	contractAddr := tx.PreAssembly.TransactionSpecification.ContractInfo.ContractAddress
	writtenStates = make([]*components.FullState, len(newStatesToWrite))
	if len(newStatesToWrite) > 0 {
		log.L(ctx).Infof("Writing %d states to domain state writer for transaction=%s domain=%s contract-address=%s", len(newStatesToWrite), tx.ID, dc.d.name, contractAddr)
		newStates, err := dsw.StageStateUpserts(ctx, readTX, newStatesToWrite...)
		if err != nil {
			return nil, err
		}

		// Store the results on the TX
		for i, s := range newStates {
			writtenStates[i] = &components.FullState{
				ID:     s.ID,
				Schema: s.Schema,
				Data:   s.Data,
			}
		}
	}
	return writtenStates, nil
}

// Endorse is a little special, because it returns a payload rather than updating the transaction.
func (dc *domainContract) EndorseTransaction(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, req *components.PrivateTransactionEndorseRequest) (*components.EndorsementResult, error) {

	if req == nil ||
		req.TransactionSpecification == nil ||
		req.Endorsement == nil ||
		req.Endorser == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainReqIncompleteEndorseTransaction)
	}

	c := dc.d.newInFlightDomainRequest(readTX, dqc, true)
	defer c.close()

	// This function does NOT FLUSH before or after doing endorse. The assumption is that this
	// is being handled as part of an overall sequence of endorsements, and for performance it is
	// more important to reduce the total number of flushes (rather than focus on the latency of one TX).
	//
	// The engine must ensure the flush occurs before returning the endorsement back to the requester,
	// but for efficiency we can and should start the runtime exercise of endorsement + signing before
	// waiting for the DB TX to commit.

	// Run the endorsement
	log.L(ctx).Infof("Running endorse transaction=%s domain=%s contract-address=%s",
		req.TransactionSpecification.TransactionId, dc.d.name, req.TransactionSpecification.ContractInfo.ContractAddress)
	res, err := dc.api.EndorseTransaction(ctx, &prototk.EndorseTransactionRequest{
		StateQueryContext:   c.id,
		Transaction:         req.TransactionSpecification,
		ResolvedVerifiers:   req.Verifiers,
		Inputs:              req.InputStates,
		Reads:               req.ReadStates,
		Outputs:             req.OutputStates,
		Info:                req.InfoStates,
		Signatures:          req.Signatures,
		EndorsementRequest:  req.Endorsement,
		EndorsementVerifier: req.Endorser,
	})
	// We don't do any processing - as the result is not directly processable by us.
	// It is an instruction to the engine - such as an authority to sign an endorsement,
	// or a constraint on submission to the chain
	if err != nil {
		return nil, err
	}
	return &components.EndorsementResult{
		Endorser:     req.Endorser,
		Result:       res.EndorsementResult,
		Payload:      res.Payload,
		RevertReason: res.RevertReason,
	}, nil
}

func (dc *domainContract) PrepareTransaction(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, tx *components.PrivateTransaction) error {
	if tx.PreAssembly == nil || tx.PreAssembly.TransactionSpecification == nil ||
		tx.PostAssembly == nil || tx.Signer == "" {
		return i18n.NewError(ctx, msgs.MsgDomainTXIncompletePrepareTransaction)
	}

	preAssembly := tx.PreAssembly
	postAssembly := tx.PostAssembly

	c := dc.d.newInFlightDomainRequest(readTX, dqc, true)
	defer c.close()

	// Run the prepare
	log.L(ctx).Info("Preparing transaction")
	res, err := dc.api.PrepareTransaction(ctx, &prototk.PrepareTransactionRequest{
		StateQueryContext: c.id,
		Transaction:       preAssembly.TransactionSpecification,
		InputStates:       dc.d.toEndorsableList(postAssembly.InputStates),
		ReadStates:        dc.d.toEndorsableList(postAssembly.ReadStates),
		OutputStates:      dc.d.toEndorsableList(postAssembly.OutputStates),
		InfoStates:        dc.d.toEndorsableList(postAssembly.InfoStates),
		AttestationResult: dc.allAttestations(tx),
		ResolvedVerifiers: postAssembly.ResolvedVerifiers,
		DomainData:        postAssembly.DomainData,
	})
	if err != nil {
		return err
	}

	var functionABI abi.Entry
	if err := json.Unmarshal(([]byte)(res.Transaction.FunctionAbiJson), &functionABI); err != nil {
		return i18n.WrapError(ctx, err, msgs.MsgDomainPrivateAbiJsonInvalid)
	}

	contractAddress := &dc.info.Address
	if res.Transaction.ContractAddress != nil {
		contractAddress, err = pldtypes.ParseEthAddress(*res.Transaction.ContractAddress)
		if err != nil {
			return err
		}
	}

	if res.Transaction.RequiredSigner != nil && len(*res.Transaction.RequiredSigner) > 0 {
		tx.Signer = *res.Transaction.RequiredSigner
	}

	if res.Transaction.Type == prototk.PreparedTransaction_PRIVATE {
		psc, err := dc.dm.GetSmartContractByAddress(ctx, readTX, *contractAddress)
		if err != nil {
			return err
		}
		tx.PreparedPrivateTransaction = &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				IdempotencyKey: fmt.Sprintf("%s_%s", tx.ID, functionABI.Name),
				Type:           pldapi.TransactionTypePrivate.Enum(),
				Function:       functionABI.String(),
				From:           tx.Signer,
				To:             contractAddress,
				Data:           pldtypes.RawJSON(res.Transaction.ParamsJson),
				Domain:         psc.Domain().Name(),
			},
			ABI: abi.ABI{&functionABI},
		}
	} else {
		tx.PreparedPublicTransaction = &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type:            pldapi.TransactionTypePublic.Enum(),
				Function:        functionABI.String(),
				From:            tx.Signer,
				To:              contractAddress,
				Data:            pldtypes.RawJSON(res.Transaction.ParamsJson),
				PublicTxOptions: tx.PreAssembly.PublicTxOptions,
			},
			ABI: abi.ABI{&functionABI},
		}
		// We cannot fall back to eth_estimateGas, as we queue up multiple transactions for dispatch that chain together.
		// As such our transactions are not always executable in isolation, and would revert (due to consuming non-existent UTXO states)
		// if we attempted to do gas estimation or call.
		if tx.PreparedPublicTransaction.Gas == nil {
			tx.PreparedPublicTransaction.Gas = &dc.d.defaultGasLimit
		}
	}
	if res.Metadata != nil {
		tx.PreparedMetadata = pldtypes.RawJSON(*res.Metadata)
	}
	return nil
}

func (dc *domainContract) InitCall(ctx context.Context, callTx *components.ResolvedTransaction) ([]*prototk.ResolveVerifierRequest, error) {

	txSpec, err := dc.buildTransactionSpecification(ctx, callTx, prototk.TransactionSpecification_CALL)
	if err != nil {
		return nil, err
	}

	// Call the domain
	res, err := dc.api.InitCall(ctx, &prototk.InitCallRequest{
		Transaction: txSpec,
	})
	if err != nil {
		return nil, err
	}

	return res.RequiredVerifiers, nil

}

func (dc *domainContract) ExecCall(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, callTx *components.ResolvedTransaction, verifiers []*prototk.ResolvedVerifier) (*abi.ComponentValue, error) {

	txSpec, err := dc.buildTransactionSpecification(ctx, callTx, prototk.TransactionSpecification_CALL)
	if err != nil {
		return nil, err
	}

	// We expect queries to the state store during this call
	c := dc.d.newInFlightDomainRequest(readTX, dqc, true)
	defer c.close()

	// Call the domain
	res, err := dc.api.ExecCall(ctx, &prototk.ExecCallRequest{
		StateQueryContext: c.id,
		ResolvedVerifiers: verifiers,
		Transaction:       txSpec,
	})
	if err != nil {
		return nil, err
	}

	// The outputs must conform to the spec
	outputDef := callTx.Function.Definition.Outputs
	if outputDef == nil {
		outputDef = abi.ParameterArray{}
	}
	if res.ResultJson == "" {
		res.ResultJson = `[]`
	}
	cv, err := outputDef.ParseJSONCtx(ctx, []byte(res.ResultJson))
	if err != nil {
		log.L(ctx).Errorf("Invalid data from domain for %s: %s", callTx.Function.Definition.SolString(), res.ResultJson)
		return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidDataFromDomain)
	}

	return cv, nil
}

func (dc *domainContract) Domain() components.Domain {
	return dc.d
}

func (dc *domainContract) Address() pldtypes.EthAddress {
	return dc.info.Address
}

func (dc *domainContract) allAttestations(tx *components.PrivateTransaction) []*prototk.AttestationResult {
	attestations := append([]*prototk.AttestationResult{}, tx.PostAssembly.Signatures...)
	attestations = append(attestations, tx.PostAssembly.Endorsements...)
	return attestations
}

func (dc *domainContract) loadStatesFromContext(ctx context.Context, dqc components.DomainQueryContext, readTX persistence.DBTX, refs []*prototk.StateRef) ([]*components.FullState, error) {
	rawIDsBySchema := make(map[pldtypes.Bytes32][]pldtypes.RawJSON)
	stateIDs := make([]pldtypes.HexBytes, len(refs))
	for i, s := range refs {
		stateID, err := pldtypes.ParseHexBytes(ctx, s.Id)
		var schemaID pldtypes.Bytes32
		if err == nil {
			schemaID, err = pldtypes.ParseBytes32(s.SchemaId)
		}
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidStateIDFromDomain, s.Id, i)
		}
		rawIDsBySchema[schemaID] = append(rawIDsBySchema[schemaID], pldtypes.JSONString(stateID.String()))
		stateIDs[i] = stateID
	}
	statesByID := make(map[string]*pldapi.State)
	for schemaID, stateIDs := range rawIDsBySchema {
		log.L(ctx).Debugf("Finding available states for state IDs %+v", stateIDs)
		_, statesForSchema, err := dqc.FindAvailableStates(ctx, readTX, schemaID, &query.QueryJSON{
			Statements: query.Statements{
				Ops: query.Ops{
					In: []*query.OpMultiVal{
						{Op: query.Op{Field: ".id"}, Values: stateIDs},
					},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		for _, s := range statesForSchema {
			statesByID[s.ID.HexString()] = s
		}
	}

	// Check we found all the states, and restore the original order
	states := make([]*components.FullState, len(stateIDs))
	for i, id := range stateIDs {
		s := statesByID[id.HexString()]
		if s == nil {
			return nil, i18n.NewError(ctx, msgs.MsgDomainInputStateNotFound, i, id)
		}
		states[i] = &components.FullState{
			ID:     s.ID,
			Schema: s.Schema,
			Data:   s.Data,
		}
	}
	log.L(ctx).Debugf("Found available states %+v", states)
	return states, nil

}

func mapPrivacyGroupToProto(stateID pldtypes.HexBytes, pg *pldapi.PrivacyGroupGenesisState) *prototk.PrivacyGroup {
	return &prototk.PrivacyGroup{
		Id:            stateID.String(),
		GenesisSalt:   pg.GenesisSalt.String(),
		Name:          pg.Name,
		Members:       pg.Members,
		Properties:    pg.Properties.Map(),
		Configuration: pg.Configuration.Map(),
	}
}

func (dc *domainContract) WrapPrivacyGroupEVMTX(ctx context.Context, pg *pldapi.PrivacyGroup, pgTX *pldapi.PrivacyGroupEVMTX) (ptx *pldapi.TransactionInput, err error) {

	// We do nothing apart from type conversion here, as the domain is going to wrap this call
	// and return the private transaction, such that it can be validated fully there.

	var pToAddr *string
	if pgTX.To != nil {
		toAddr := pgTX.To.String()
		pToAddr = &toAddr
	}
	var pGas *string
	if pgTX.Gas != nil {
		gasStr := pgTX.Gas.String()
		pGas = &gasStr
	}
	var pValue *string
	if pgTX.Value != nil {
		valueStr := pgTX.Value.String()
		pValue = &valueStr
	}
	var pInput *string
	if pgTX.Input != nil {
		inputStr := pgTX.Input.String()
		pInput = &inputStr
	}
	var pABI *string
	if pgTX.Function != nil {
		abiStr := pldtypes.JSONString(pgTX.Function).String()
		pABI = &abiStr
	}

	// Call the domain to do the work
	res, err := dc.api.WrapPrivacyGroupEVMTX(ctx, &prototk.WrapPrivacyGroupEVMTXRequest{
		PrivacyGroup: mapPrivacyGroupToProto(pg.ID, pg.GenesisStateData()),
		Transaction: &prototk.PrivacyGroupEVMTX{
			ContractInfo: &prototk.ContractInfo{
				ContractAddress:    dc.info.Address.String(),
				ContractConfigJson: dc.config.ContractConfigJson,
			},
			From:            pgTX.From,
			To:              pToAddr,
			Gas:             pGas,
			Value:           pValue,
			InputJson:       pInput,
			FunctionAbiJson: pABI,
			Bytecode:        pgTX.Bytecode,
		},
	})
	if err != nil {
		return nil, err
	}

	// Function returned must be private
	if res.Transaction.Type != prototk.PreparedTransaction_PRIVATE {
		return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidPGroupTxTypeNotPrivate, res.Transaction.Type)
	}

	ptx = &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:   pldapi.TransactionTypePrivate.Enum(),
			Domain: dc.d.name,
		},
	}

	pscAddr := dc.Address()
	if res.Transaction.ContractAddress != nil {
		addr, err := pldtypes.ParseEthAddress(*res.Transaction.ContractAddress)
		if err != nil {
			return nil, err
		}
		if *addr != pscAddr {
			return nil, i18n.NewError(ctx, msgs.MsgDomainInvalidPGroupTxCannotRedirect, pscAddr, addr)
		}
	}
	ptx.To = &pscAddr

	// Always set the function definition
	var wrappedFnABI abi.Entry
	if err := json.Unmarshal([]byte(res.Transaction.FunctionAbiJson), &wrappedFnABI); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgDomainInvalidFunctionParams, res.Transaction.FunctionAbiJson)
	}
	ptx.Function = wrappedFnABI.Name
	ptx.ABIReference = nil
	ptx.ABI = abi.ABI{&wrappedFnABI}

	// And the inputs
	ptx.Data = pldtypes.RawJSON(res.Transaction.ParamsJson)

	// Only update the signer if returned
	if res.Transaction.RequiredSigner != nil && len(*res.Transaction.RequiredSigner) > 0 {
		ptx.From = *res.Transaction.RequiredSigner
	}

	return ptx, nil

}
func (dc *domainContract) IsBaseLedgerRevertRetryable(ctx context.Context, revertData []byte) (bool, string, error) {
	res, err := dc.api.IsBaseLedgerRevertRetryable(ctx, &prototk.IsBaseLedgerRevertRetryableRequest{
		RevertData: revertData,
	})
	if err != nil {
		return false, "", err
	}
	return res.Retryable, res.DecodedReason, nil
}

func (dc *domainContract) InvokeRPC(ctx context.Context, dqc components.DomainQueryContext, dbTX persistence.DBTX, rpcCall pldapi.DomainInvokeRPC) (pldtypes.RawJSON, error) {
	c := dc.d.newInFlightDomainRequest(dbTX, dqc, false)
	defer c.close()
	res, err := dc.api.InvokeRPC(ctx, &prototk.InvokeRPCRequest{
		StateQueryContext: c.id,
		Method:            rpcCall.Method,
		ParamsJson:        string(rpcCall.Params),
	})
	if err != nil {
		return nil, err
	}
	return pldtypes.RawJSON(res.ResultJson), nil
}
