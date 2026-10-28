// SPDX-License-Identifier: CC0-1.0
pragma solidity ^0.8.20;

interface IERC8316 {
    struct LockInfo {
        address owner;
        address spender;
        bytes32 spendCommitment;
        bytes32 cancelCommitment;
    }

    event LockCreated(
        bytes32 indexed lockId,
        address indexed owner,
        address indexed spender,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes data
    );

    event LockUpdated(
        bytes32 indexed lockId,
        address indexed owner,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes data
    );

    event LockDelegated(
        bytes32 indexed lockId,
        address indexed previousSpender,
        address indexed newSpender,
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
    error LockImmutable(bytes32 lockId);
    error LockInvalidSpender(bytes32 lockId, address spender);

    function createLock(
        bytes calldata createArgs,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external returns (bytes32 lockId);

    function updateLock(
        bytes32 lockId,
        bytes calldata updateArgs,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external;

    function delegateLock(
        bytes32 lockId,
        bytes calldata delegateArgs,
        address newSpender,
        bytes calldata data
    ) external;

    function spendLock(
        bytes32 lockId,
        bytes calldata spendArgs,
        bytes calldata data
    ) external;

    function cancelLock(
        bytes32 lockId,
        bytes calldata cancelArgs,
        bytes calldata data
    ) external;

    function getLock(bytes32 lockId) external view returns (LockInfo memory info);

    function isLockActive(bytes32 lockId) external view returns (bool active);
}
