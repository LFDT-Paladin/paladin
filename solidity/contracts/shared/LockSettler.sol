// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {ReentrancyGuardUpgradeable} from "@openzeppelin/contracts-upgradeable/utils/ReentrancyGuardUpgradeable.sol";
import {ILockSettler} from "./interfaces/ILockSettler.sol";
import {ILockableCapability} from "../domains/interfaces/ILockableCapability.sol";

/**
 * @title LockSettler
 * @dev Atomically spends or cancels a set of ILockableCapability locks.
 */
contract LockSettler is
    Initializable,
    ReentrancyGuardUpgradeable,
    ILockSettler
{
    Status public status;
    LockEntry[] private _lockEntries;

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    /**
     * Initialize the LockSettler with a list of lock entries.
     * @param lockEntries The locks to spend or cancel atomically
     */
    function initialize(LockEntry[] memory lockEntries) external initializer {
        __ReentrancyGuard_init();
        status = Status.Pending;

        for (uint256 i = 0; i < lockEntries.length; i++) {
            _lockEntries.push(lockEntries[i]);
        }

        emit LockSettlerStatusChanged(status);
    }

    /**
     * @dev Spend all locks atomically.
     * Reverts if the LockSettler has already been spent or cancelled, or if any spendLock call fails.
     */
    function spend() external nonReentrant {
        if (status != Status.Pending) {
            revert LockSettlerNotPending();
        }

        for (uint256 i = 0; i < _lockEntries.length; i++) {
            LockEntry storage entry = _lockEntries[i];
            ILockableCapability(entry.contractAddress).spendLock(
                entry.lockId,
                entry.spendInputs,
                entry.data
            );
        }

        status = Status.Spent;
        emit LockSettlerStatusChanged(status);
    }

    /**
     * @dev Cancel all locks, preventing their spending.
     * Individual cancel operations may fail without reverting the entire cancel (idempotent).
     * Can be called multiple times if already Cancelled - will retry cancel operations each time.
     */
    function cancel() external nonReentrant {
        if (status == Status.Spent) {
            revert LockSettlerNotPending();
        }

        for (uint256 i = 0; i < _lockEntries.length; i++) {
            LockEntry storage entry = _lockEntries[i];
            (bool success, ) = entry.contractAddress.call(
                abi.encodeCall(
                    ILockableCapability.cancelLock,
                    (entry.lockId, entry.cancelInputs, entry.data)
                )
            );
            if (!success) {
                // intentionally continue even if individual cancels fail (idempotent)
            }
        }

        if (status != Status.Cancelled) {
            status = Status.Cancelled;
            emit LockSettlerStatusChanged(status);
        }
    }

    /**
     * @dev Simulate the spending of all locks.
     * This function always reverts with the encoded results of the spendLock calls
     * (even if all calls succeed). It should only be used with eth_call,
     * as a transaction with this method will always revert.
     */
    function simulate() external nonReentrant {
        if (status != Status.Pending) {
            revert LockSettlerNotPending();
        }

        CallResult[] memory results = new CallResult[](_lockEntries.length);
        for (uint256 i = 0; i < _lockEntries.length; i++) {
            LockEntry storage entry = _lockEntries[i];
            (results[i].success, results[i].returnData) = entry
                .contractAddress
                .call(
                    abi.encodeCall(
                        ILockableCapability.spendLock,
                        (entry.lockId, entry.spendInputs, entry.data)
                    )
                );
        }
        revert SpendResult(results);
    }

    function getLockCount() external view returns (uint256) {
        return _lockEntries.length;
    }

    function getLock(uint256 n) external view returns (LockEntry memory) {
        return _lockEntries[n];
    }

    function getLocks() external view returns (LockEntry[] memory) {
        return _lockEntries;
    }
}
