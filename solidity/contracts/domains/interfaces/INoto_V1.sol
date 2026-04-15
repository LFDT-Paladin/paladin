// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IConfidentialToken} from "../interfaces/IConfidentialToken.sol";

/**
 * @title INoto_V1
 * @dev Backwards-compatible V1 ABI surface for Noto.
 */
interface INoto_V1 is IConfidentialToken {
    struct LockInfo {
        address owner;
        bytes content;
        address spender;
        bytes32 spendHash;
        bytes32 cancelHash;
        bytes options;
    }

    struct NotoCreateLockOperation {
        bytes32 txId;
        bytes32[] inputs;
        bytes32[] outputs;
        bytes32[] contents;
        bytes32 newLockState;
        bytes proof;
    }

    struct NotoUpdateLockOperation {
        bytes32 txId;
        bytes32 oldLockState;
        bytes32 newLockState;
        bytes proof;
    }

    struct NotoUnlockOperation {
        bytes32 txId;
        bytes32[] inputs;
        bytes32[] outputs;
        bytes data;
        bytes proof;
    }

    struct NotoDelegateOperation {
        bytes32 txId;
        bytes32 oldLockState;
        bytes32 newLockState;
        bytes proof;
    }

    struct NotoLockOptions {
        bytes32 spendTxId;
    }

    struct LockParams {
        bytes32 spendHash;
        bytes32 cancelHash;
        bytes options;
    }

    event LockUpdated(
        bytes32 indexed lockId,
        address operator,
        LockInfo lock,
        bytes data
    );

    event LockDelegated(
        bytes32 indexed lockId,
        address indexed from,
        address indexed to,
        address operator,
        bytes data
    );

    event LockSpent(
        bytes32 indexed lockId,
        address indexed spender,
        bytes data
    );

    event LockCancelled(
        bytes32 indexed lockId,
        address indexed spender,
        bytes data
    );

    error LockNotActive(bytes32 lockId);
    error LockUnauthorized(bytes32 lockId, address spender, address caller);
    error LockSpenderNotOwner(bytes32 lockId, address spender, address owner);

    event NotoLockCreated(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed owner,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes32[] contents,
        bytes32 newLockState,
        bytes proof,
        bytes data
    );

    event NotoLockUpdated(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed operator,
        bytes32[] contents,
        bytes32 oldLockState,
        bytes32 newLockState,
        bytes proof,
        bytes data
    );

    event NotoLockSpent(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed spender,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes txData,
        bytes32 oldLockState,
        bytes proof,
        bytes data
    );

    event NotoLockCancelled(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed spender,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes txData,
        bytes32 oldLockState,
        bytes proof,
        bytes data
    );

    event NotoLockDelegated(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed from,
        address to,
        bytes32 oldLockState,
        bytes32 newLockState,
        bytes proof,
        bytes data
    );

    function initialize(
        string memory name_,
        string memory symbol_,
        address notary
    ) external;

    function buildConfig(
        bytes calldata data
    ) external view returns (bytes memory);

    function computeLockId(
        bytes calldata createInputs
    ) external view returns (bytes32 lockId);

    function getLockId(bytes32 id) external view returns (bytes32 lockId);

    function createLock(
        bytes calldata createInputs,
        LockParams calldata params,
        bytes calldata data
    ) external returns (bytes32 lockId);

    function updateLock(
        bytes32 lockId,
        bytes calldata updateInputs,
        LockParams calldata params,
        bytes calldata data
    ) external;

    function delegateLock(
        bytes32 lockId,
        bytes calldata delegateInputs,
        address newSpender,
        bytes calldata data
    ) external;

    function spendLock(
        bytes32 lockId,
        bytes calldata spendInputs,
        bytes calldata data
    ) external;

    function cancelLock(
        bytes32 lockId,
        bytes calldata cancelInputs,
        bytes calldata data
    ) external;

    function getLock(
        bytes32 lockId
    ) external view returns (LockInfo memory info);
}
