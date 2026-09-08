// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {INotoHooks} from "../domains/interfaces/INotoHooks.sol";
import {IRevertableTarget} from "./RevertableTarget.sol";

/**
 * @title NotoHooksRevertable
 * @dev Minimal INotoHooks implementation for testing. onTransfer can:
 *      - revert directly for the configured revertAddress
 *      - emit a revertable external call for the configured failAddress
 *      - emit an external call that reverts with NotoInvalidInput(bytes32) for invalidInputAddress
 *      - emit only the prepared transaction for all other transfers
 *      All other hook methods emit only the prepared transaction.
 */
contract NotoHooksRevertable is INotoHooks {
    address public revertableTarget;
    address public revertAddress;
    address public failAddress;
    address public invalidInputAddress;

    constructor(address _revertableTarget, address _revertAddress, address _failAddress, address _invalidInputAddress) {
        revertableTarget = _revertableTarget;
        revertAddress = _revertAddress;
        failAddress = _failAddress;
        invalidInputAddress = _invalidInputAddress;
    }

    function _emitPrepared(
        PreparedTransaction calldata prepared
    ) internal {
        emit PenteExternalCall(
            prepared.contractAddress,
            prepared.encodedCall
        );
    }

    function _emitPreparedAndRevertable(
        PreparedTransaction calldata prepared
    ) internal {
        emit PenteExternalCall(
            revertableTarget,
            abi.encodeCall(IRevertableTarget.check, ())
        );
        _emitPrepared(prepared);
    }

    function _emitPreparedAndNotoInvalidInput(
        PreparedTransaction calldata prepared
    ) internal {
        emit PenteExternalCall(
            revertableTarget,
            abi.encodeCall(IRevertableTarget.checkNotoInvalidInput, ())
        );
        _emitPrepared(prepared);
    }

    function onMint(
        address,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onTransfer(
        address,
        address,
        address to,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        if (to == revertAddress) {
            revert("Configured to revert");
        }
        if (to == failAddress) {
            _emitPreparedAndRevertable(prepared);
            return;
        }
        if (to == invalidInputAddress) {
            _emitPreparedAndNotoInvalidInput(prepared);
            return;
        }
        _emitPrepared(prepared);
    }

    function onBurn(
        address,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onLock(
        address,
        bytes32,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onCreateMintLock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onPrepareMintUnlock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onPrepareBurnUnlock(
        address,
        bytes32,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onUnlock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onPrepareUnlock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onCreateTransferLock(
        address,
        bytes32,
        address,
        uint256,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onCreateBurnLock(
        address,
        bytes32,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function onDelegateLock(
        address,
        bytes32,
        address,
        PreparedTransaction calldata prepared
    ) external override {
        _emitPrepared(prepared);
    }

    function handleDelegateUnlock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata
    ) external override {
        // No-op: this method must never revert
    }
}
