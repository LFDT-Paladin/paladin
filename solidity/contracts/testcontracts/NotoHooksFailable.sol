// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {INotoHooks} from "../domains/interfaces/INotoHooks.sol";
import {IFailableTarget} from "./FailableTarget.sol";

/**
 * @title NotoHooksFailable
 * @dev Minimal INotoHooks implementation for testing. Each hook emits a
 *      PenteExternalCall to a configurable FailableTarget contract (which can
 *      be set to revert), followed by the standard Noto prepared transaction.
 *      No ERC20 tracking or other state is maintained.
 */
contract NotoHooksFailable is INotoHooks {
    address public failableTarget;

    constructor(address _failableTarget) {
        failableTarget = _failableTarget;
    }

    function _emitCalls(
        PreparedTransaction calldata prepared
    ) internal {
        emit PenteExternalCall(
            prepared.contractAddress,
            prepared.encodedCall
        );
        emit PenteExternalCall(
            failableTarget,
            abi.encodeCall(IFailableTarget.check, ())
        );
    }

    function onMint(
        address,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitCalls(prepared);
    }

    function onTransfer(
        address,
        address,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitCalls(prepared);
    }

    function onBurn(
        address,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitCalls(prepared);
    }

    function onLock(
        address,
        bytes32,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitCalls(prepared);
    }

    function onCreateMintLock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitCalls(prepared);
    }

    function onPrepareBurnUnlock(
        address,
        bytes32,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitCalls(prepared);
    }

    function onUnlock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitCalls(prepared);
    }

    function onPrepareUnlock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitCalls(prepared);
    }

    function onDelegateLock(
        address,
        bytes32,
        address,
        PreparedTransaction calldata prepared
    ) external override {
        _emitCalls(prepared);
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
