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

package noto

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// preparedExecCommon implements the notary-triggered execution of a PREPARED lock's
// prearranged spend or cancel operation. Unlike unlock (which spends an UNPREPARED lock to
// arbitrary recipients), these operations execute the exact outcome that was fixed at prepare
// time (prepareUnlock/createTransferLock/createMintLock/createBurnLock), allowing a lock owner
// to request the notary to spend or cancel the lock on their behalf without delegation.
type preparedExecCommon struct {
	noto *Noto
}

// preparedExecParams is the common shape of the spendLock/cancelLock private method params.
type preparedExecParams struct {
	LockID pldtypes.Bytes32
	Data   pldtypes.HexBytes
}

func (h *preparedExecCommon) validateParams(ctx context.Context, config *types.NotoParsedConfig, lockID pldtypes.Bytes32) error {
	if config.IsV0() {
		// V0 has no prepared spend/cancel split
		return i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, "0")
	}
	if lockID.IsZero() {
		return i18n.NewError(ctx, msgs.MsgParameterRequired, "lockId")
	}
	return nil
}

func (h *preparedExecCommon) init(ctx context.Context, tx *types.ParsedTransaction) (*prototk.InitTransactionResponse, error) {
	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(tx.DomainConfig.NotaryLookup, tx.Transaction.From),
	}, nil
}

// preparedOutputs returns the prearranged output IDs and operation data for the requested
// outcome (spend or cancel).
func preparedOutputIDs(lockInfo *types.NotoLockInfo_V1, isCancel bool) ([]pldtypes.Bytes32, pldtypes.HexBytes) {
	if isCancel {
		return lockInfo.CancelOutputs, lockInfo.CancelData
	}
	return lockInfo.SpendOutputs, lockInfo.SpendData
}

// loadPreparedLock loads the lock and validates it can be executed by the notary on behalf of
// the requesting owner. Returns a REVERT response (not an error) for user-correctable conditions.
func (h *preparedExecCommon) loadPreparedLock(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest, ids *resolvedIdentities, lockID pldtypes.Bytes32) (*loadedLockInfo, *prototk.AssembleTransactionResponse, error) {
	existingLock, revert, err := h.noto.loadLockInfoV1(ctx, req.StateQueryContext, lockID)
	if res, err := assembleRevertOrError(revert, err); res != nil || err != nil {
		return nil, res, err
	}
	lockInfo := existingLock.lockInfo

	// The lock must be prepared (it has a prearranged operation, identified by a spend tx ID).
	if lockInfo.SpendTxId.IsZero() {
		return nil, assembleRevert(i18n.NewError(ctx, msgs.MsgLockNotPrepared)), nil
	}
	// The lock must not be delegated - the base ledger is onlySpender, so once the spender is
	// no longer the owner only the delegate can submit the operation directly.
	if !lockInfo.Spender.Equals(lockInfo.Owner) {
		return nil, assembleRevert(i18n.NewError(ctx, msgs.MsgLockDelegated, lockInfo.Spender)), nil
	}
	// Only the lock owner may request the notary to execute the prearranged operation.
	if !ids.sender.address.Equals(lockInfo.Owner) {
		return nil, assembleRevert(i18n.NewError(ctx, msgs.MsgPreparedExecOnlyOwner, lockInfo.Owner, ids.sender.address)), nil
	}
	// In hooks notary mode, the prearranged spend/cancel hooks only exist from V2 onwards.
	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() && !tx.DomainConfig.IsV2() {
		return nil, assembleRevert(i18n.NewError(ctx, msgs.MsgPreparedExecRequiresV2Hooks)), nil
	}
	return existingLock, nil, nil
}

func (h *preparedExecCommon) assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest, lockID pldtypes.Bytes32) (*prototk.AssembleTransactionResponse, error) {
	ids, err := resolveIdentities(ctx, h.noto, tx, req, "", "")
	if err != nil {
		return nil, err
	}

	existingLock, revert, err := h.loadPreparedLock(ctx, tx, req, ids, lockID)
	if revert != nil || err != nil {
		return revert, err
	}

	// Gather ALL the locked coins for this lock - the base ledger requires the operation to
	// consume the entire lock (lockedStateCount == inputs.length).
	lockedInputs, revertRes, err := h.noto.prepareLockedInputs(ctx, req.StateQueryContext, lockID, existingLock.lockInfo.Owner, big.NewInt(0), true)
	if res, err := assembleRevertOrError(revertRes, err); res != nil || err != nil {
		return res, err
	}

	// Consume the locked coins and the lock state. The prearranged output coins were already
	// distributed at prepare time, and are confirmed by the resulting NotoLockSpent/NotoLockCancelled
	// event - exactly as in the existing delegate-execution flow.
	assembledTransaction := &prototk.AssembledTransaction{
		InputStates: append(lockedInputs.states, existingLock.stateRef),
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult:       prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: assembledTransaction,
		// Nothing new is authorized here - the operation was authorized by the owner at prepare
		// time - so the notary simply endorses and submits (no fresh sender signature).
		AttestationPlan: []*prototk.AttestationRequest{
			buildNotaryEndorsement(tx.DomainConfig.NotaryLookup),
		},
	}, nil
}

func (h *preparedExecCommon) endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest, lockID pldtypes.Bytes32) (*prototk.EndorseTransactionResponse, error) {
	// Validate the lock-spend transition (consumes the input lock state, no output lock state)
	lt, err := h.noto.validateV1LockTransition(ctx, LOCK_SPEND, nil /* notary submits, not the owner */, &lockID, req.Inputs, req.Outputs)
	if err != nil {
		return nil, err
	}
	// Re-check the lock is prepared and undelegated, and that the requester is the owner
	if lt.prevLockInfo.SpendTxId.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgLockNotPrepared)
	}
	if !lt.prevLockInfo.Spender.Equals(lt.prevLockInfo.Owner) {
		return nil, i18n.NewError(ctx, msgs.MsgLockDelegated, lt.prevLockInfo.Spender)
	}
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	if !senderID.address.Equals(lt.prevLockInfo.Owner) {
		return nil, i18n.NewError(ctx, msgs.MsgPreparedExecOnlyOwner, lt.prevLockInfo.Owner, senderID.address)
	}

	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *preparedExecCommon) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, lockID pldtypes.Bytes32, outerData pldtypes.HexBytes, isCancel bool) (*TransactionWrapper, error) {
	// Recover the prepared lock from the input lock state
	lt, err := h.noto.validateV1LockTransition(ctx, LOCK_SPEND, nil, &lockID, req.InputStates, req.OutputStates)
	if err != nil {
		return nil, err
	}
	lockInfo := &lt.prevLockInfo
	outputIDs, opData := preparedOutputIDs(lockInfo, isCancel)
	lockedInputIDs := endorsableStateIDs(h.noto.filterSchema(req.InputStates, []string{h.noto.lockedCoinSchema.Id}))

	// Replay the exact prearranged operation so the base-ledger commitment hash matches.
	opArgs, err := h.noto.encodeNotoSpendLockArgs(ctx, &types.NotoSpendLockArgs{
		TxId:    lockInfo.SpendTxId.String(),
		Inputs:  lockedInputIDs,
		Outputs: stringIDs(outputIDs),
		Data:    opData,
		Proof:   pldtypes.HexBytes{}, // the proof was supplied at prepare time
	})
	if err != nil {
		return nil, err
	}

	interfaceABI := h.noto.getInterfaceABI(tx.DomainConfig.Variant)
	var functionName string
	var paramsJSON []byte
	if isCancel {
		functionName = "cancelLock"
		paramsJSON, err = json.Marshal(&CancelLockParams{
			LockID:     lockID,
			CancelArgs: opArgs,
			Data:       outerData,
		})
	} else {
		functionName = "spendLock"
		paramsJSON, err = json.Marshal(&SpendLockParams{
			LockID:    lockID,
			SpendArgs: opArgs,
			Data:      outerData,
		})
	}
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: interfaceABI.Functions()[functionName],
		paramsJSON:  paramsJSON,
	}, nil
}

// resolvedRecipientsFromOutputs reconstructs the recipient address/amount list from the
// prearranged output coins (needed for the V2 onSpendLock hook).
func (h *preparedExecCommon) resolvedRecipientsFromOutputs(ctx context.Context, stateQueryContext string, outputIDs []pldtypes.Bytes32) ([]*ResolvedUnlockRecipient, error) {
	if len(outputIDs) == 0 {
		return []*ResolvedUnlockRecipient{}, nil
	}
	states, err := h.noto.getStates(ctx, stateQueryContext, h.noto.coinSchema.Id, stringIDs(outputIDs))
	if err != nil {
		return nil, err
	}
	if len(states) != len(outputIDs) {
		return nil, i18n.NewError(ctx, msgs.MsgMissingStateData, stringIDs(outputIDs))
	}
	recipients := make([]*ResolvedUnlockRecipient, len(states))
	for i, state := range states {
		coin, err := h.noto.unmarshalCoin(state.DataJson)
		if err != nil {
			return nil, err
		}
		recipients[i] = &ResolvedUnlockRecipient{To: coin.Owner, Amount: coin.Amount}
	}
	return recipients, nil
}

func (h *preparedExecCommon) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper, lockID pldtypes.Bytes32, outerData pldtypes.HexBytes, isCancel bool) (*TransactionWrapper, error) {
	senderID, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	prepared := PreparedTransaction{
		ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
		EncodedCall:     encodedCall,
	}

	var functionName string
	var params any
	if isCancel {
		functionName = "onCancelLock"
		params = &CancelLockHookParams{
			Sender:   senderID.address,
			LockID:   lockID,
			Data:     outerData,
			Prepared: prepared,
		}
	} else {
		lt, err := h.noto.validateV1LockTransition(ctx, LOCK_SPEND, nil, &lockID, req.InputStates, req.OutputStates)
		if err != nil {
			return nil, err
		}
		recipients, err := h.resolvedRecipientsFromOutputs(ctx, req.StateQueryContext, lt.prevLockInfo.SpendOutputs)
		if err != nil {
			return nil, err
		}
		functionName = "onSpendLock"
		params = &UnlockHookParams{
			Sender:     senderID.address,
			LockID:     lockID,
			Recipients: recipients,
			Data:       outerData,
			Prepared:   prepared,
		}
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksV2Build.ABI.Functions()[functionName],
		params,
	)
	if err != nil {
		return nil, err
	}

	return &TransactionWrapper{
		transactionType: mapPrepareTransactionType(transactionType),
		functionABI:     functionABI,
		paramsJSON:      paramsJSON,
		contractAddress: tx.DomainConfig.Options.Hooks.PublicAddress,
	}, nil
}

func (h *preparedExecCommon) prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, lockID pldtypes.Bytes32, outerData pldtypes.HexBytes, isCancel bool) (*prototk.PrepareTransactionResponse, error) {
	endorsement := domain.FindAttestation("notary", req.AttestationResult)
	if endorsement == nil || endorsement.Verifier.Lookup != tx.DomainConfig.NotaryLookup {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "notary")
	}

	baseTransaction, err := h.baseLedgerInvoke(ctx, tx, req, lockID, outerData, isCancel)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() {
		hookTransaction, err := h.hookInvoke(ctx, tx, req, baseTransaction, lockID, outerData, isCancel)
		if err != nil {
			return nil, err
		}
		return hookTransaction.prepare()
	}

	return baseTransaction.prepare()
}

// --- spendLock handler ---

type spendLockHandler struct {
	preparedExecCommon
}

func (h *spendLockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var p types.SpendLockParams
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return nil, err
	}
	if err := h.validateParams(ctx, config, p.LockID); err != nil {
		return nil, err
	}
	return &p, nil
}

func (h *spendLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	return h.init(ctx, tx)
}

func (h *spendLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.SpendLockParams)
	return h.assemble(ctx, tx, req, params.LockID)
}

func (h *spendLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.SpendLockParams)
	return h.endorse(ctx, tx, req, params.LockID)
}

func (h *spendLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	params := tx.Params.(*types.SpendLockParams)
	return h.prepare(ctx, tx, req, params.LockID, params.Data, false)
}

// --- cancelLock handler ---

type cancelLockHandler struct {
	preparedExecCommon
}

func (h *cancelLockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var p types.CancelLockParams
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return nil, err
	}
	if err := h.validateParams(ctx, config, p.LockID); err != nil {
		return nil, err
	}
	return &p, nil
}

func (h *cancelLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	return h.init(ctx, tx)
}

func (h *cancelLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.CancelLockParams)
	return h.assemble(ctx, tx, req, params.LockID)
}

func (h *cancelLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.CancelLockParams)
	return h.endorse(ctx, tx, req, params.LockID)
}

func (h *cancelLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	params := tx.Params.(*types.CancelLockParams)
	return h.prepare(ctx, tx, req, params.LockID, params.Data, true)
}
