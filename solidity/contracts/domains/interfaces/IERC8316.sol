// SPDX-License-Identifier: CC0-1.0
pragma solidity ^0.8.20;

/**
 * @title IERC8316
 * @dev Canonical interface for ERC-8316: Programmable Settlement Locks.
 *
 *      A programmable settlement lock is a prepared value-bearing operation
 *      controlled by a single current spender, with predefined spend and
 *      cancel paths secured by implementation-specific commitments. Only the
 *      current spender may spend or cancel the lock, and only according to the
 *      paths prepared by the lock owner.
 *
 *      This interface standardizes the lock lifecycle and authority model. The
 *      construction and validation of `spendCommitment` and `cancelCommitment`,
 *      the encoding of the `*Args` payloads, and the meaning of the underlying
 *      operation are all implementation-specific.
 *
 *      See https://ethereum-magicians.org/t/erc-8316-programmable-settlement-locks/28861
 */
interface IERC8316 {
    /**
     * @dev Stored lock information, queryable by lockId.
     */
    struct LockInfo {
        /// @dev The address that created the lock. Immutable for the lock's lifetime.
        address owner;
        /// @dev The address currently authorized to spend, cancel, or delegate the lock.
        address spender;
        /// @dev Implementation-defined commitment to the spend path. Mutable only while `spender == owner`.
        bytes32 spendCommitment;
        /// @dev Implementation-defined commitment to the cancel path. Mutable only while `spender == owner`.
        bytes32 cancelCommitment;
    }

    /// @dev Emitted when a lock is created.
    event LockCreated(
        bytes32 indexed lockId,
        address indexed owner,
        address indexed spender,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes data
    );

    /// @dev Emitted when an active, owner-controlled lock is updated.
    event LockUpdated(
        bytes32 indexed lockId,
        address indexed owner,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes data
    );

    /// @dev Emitted when spending authority is delegated from one address to another.
    event LockDelegated(
        bytes32 indexed lockId,
        address indexed previousSpender,
        address indexed newSpender,
        bytes data
    );

    /// @dev Emitted when a lock is successfully spent.
    event LockSpent(bytes32 indexed lockId, address indexed spender, bytes data);

    /// @dev Emitted when a lock is successfully cancelled.
    event LockCancelled(bytes32 indexed lockId, address indexed spender, bytes data);

    /// @dev Thrown when a lock does not exist or is not active.
    error LockNotActive(bytes32 lockId);

    /// @dev Thrown when the caller is not the current spender for the given lock.
    error LockUnauthorized(bytes32 lockId, address spender, address caller);

    /// @dev Thrown when an attempted mutation is not permitted under the lock's mutability rules.
    error LockImmutable(bytes32 lockId);

    /// @dev Thrown when a delegation target is invalid (e.g. the zero address).
    error LockInvalidSpender(bytes32 lockId, address spender);

    /**
     * @dev Create a new active lock.
     *
     * Requirements:
     *  - MUST create a new active lock with a unique lockId
     *  - MUST set owner and spender to msg.sender
     *  - MUST initialize the commitments to the provided values
     *  - MUST emit {LockCreated}
     *
     * @param createArgs       Implementation-specific calldata needed to create the lock.
     * @param spendCommitment  Commitment to the spend path.
     * @param cancelCommitment Commitment to the cancel path.
     * @param data             Auxiliary application-specific calldata, passed through to the event.
     * @return lockId          The generated unique identifier for the lock.
     */
    function createLock(
        bytes calldata createArgs,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external returns (bytes32 lockId);

    /**
     * @dev Update the commitments of an active, owner-controlled lock.
     *
     * Requirements:
     *  - MUST revert with {LockNotActive} if the lock is not active
     *  - MUST revert with {LockUnauthorized} if msg.sender is not the current spender
     *  - MUST revert with {LockImmutable} unless `spender == owner`
     *  - MUST emit {LockUpdated}
     */
    function updateLock(
        bytes32 lockId,
        bytes calldata updateArgs,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external;

    /**
     * @dev Delegate spending authority for this lock to a new address.
     *
     * Requirements:
     *  - MUST revert with {LockNotActive} if the lock is not active
     *  - MUST revert with {LockUnauthorized} if msg.sender is not the current spender
     *  - MUST revert with {LockInvalidSpender} if newSpender == address(0)
     *  - MUST update the current spender to newSpender
     *  - MUST emit {LockDelegated}
     */
    function delegateLock(
        bytes32 lockId,
        bytes calldata delegateArgs,
        address newSpender,
        bytes calldata data
    ) external;

    /**
     * @dev Spend the lock, executing its prepared operation.
     *
     * Requirements:
     *  - MUST revert with {LockNotActive} if the lock is not active
     *  - MUST revert with {LockUnauthorized} if msg.sender is not the current spender
     *  - MUST succeed if active, caller is the current spender, and spendArgs validate against spendCommitment
     *  - MUST transition the lock to inactive
     *  - MUST emit {LockSpent}
     */
    function spendLock(
        bytes32 lockId,
        bytes calldata spendArgs,
        bytes calldata data
    ) external;

    /**
     * @dev Cancel the lock, executing its prepared cancellation path.
     *
     * Requirements:
     *  - MUST revert with {LockNotActive} if the lock is not active
     *  - MUST revert with {LockUnauthorized} if msg.sender is not the current spender
     *  - MUST succeed if active, caller is the current spender, and cancelArgs validate against cancelCommitment
     *  - MUST transition the lock to inactive
     *  - MUST emit {LockCancelled}
     */
    function cancelLock(
        bytes32 lockId,
        bytes calldata cancelArgs,
        bytes calldata data
    ) external;

    /**
     * @dev Return stored information for an active lock.
     *  - MUST revert with {LockNotActive} if the lock is not active.
     */
    function getLock(bytes32 lockId) external view returns (LockInfo memory info);

    /**
     * @dev Return true if and only if the lock is active.
     */
    function isLockActive(bytes32 lockId) external view returns (bool active);
}
