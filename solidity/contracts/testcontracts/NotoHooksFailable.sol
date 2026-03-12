// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {INotoHooks} from "../domains/interfaces/INotoHooks.sol";
import {IFailableTarget} from "./FailableTarget.sol";

/**
 * @title NotoHooksFailable
 * @dev Minimal INotoHooks implementation for testing. Most hooks emit a
 *      PenteExternalCall to a configurable FailableTarget contract (which can
 *      be set to revert), followed by the standard Noto prepared transaction.
 *      Mint only emits the prepared transaction (no failable call), so that
 *      initial funding cannot be disrupted by the failure mechanism.
 */
contract NotoHooksFailable is INotoHooks {
    address public failableTarget;

    constructor(address _failableTarget) {
        failableTarget = _failableTarget;
    }

    function _emitPrepared(
        PreparedTransaction calldata prepared
    ) internal {
        emit PenteExternalCall(
            prepared.contractAddress,
            prepared.encodedCall
        );
    }

    function _emitFailableAndPrepared(
        PreparedTransaction calldata prepared
    ) internal {
        _emitPrepared(prepared);
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
        _emitPrepared(prepared);
    }

    function onTransfer(
        address,
        address,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitFailableAndPrepared(prepared);
    }

    function onBurn(
        address,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitFailableAndPrepared(prepared);
    }

    function onLock(
        address,
        bytes32,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitFailableAndPrepared(prepared);
    }

    function onCreateMintLock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitFailableAndPrepared(prepared);
    }

    function onPrepareBurnUnlock(
        address,
        bytes32,
        address,
        uint256,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitFailableAndPrepared(prepared);
    }

    function onUnlock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitFailableAndPrepared(prepared);
    }

    function onPrepareUnlock(
        address,
        bytes32,
        UnlockRecipient[] calldata,
        bytes calldata,
        PreparedTransaction calldata prepared
    ) external override {
        _emitFailableAndPrepared(prepared);
    }

    function onDelegateLock(
        address,
        bytes32,
        address,
        PreparedTransaction calldata prepared
    ) external override {
        _emitFailableAndPrepared(prepared);
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
