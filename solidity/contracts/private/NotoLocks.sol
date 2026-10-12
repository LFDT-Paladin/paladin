// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {INotoHooks} from "../domains/interfaces/INotoHooks.sol";

/**
 * Helpers for tracking locked amounts from Noto hooks contracts.
 */
contract NotoLocks {
    // Details on all currently active locks and their possible unlocks
    mapping(bytes32 => LockDetail) internal _locks;

    // Balances locked by address (still logically owned by that address)
    mapping(address => uint256) public lockedBalance;

    // Pending balances from prepared unlocks (not yet owned, but approved to be owned when unlocked)
    mapping(address => uint256) public pendingBalance;

    // What a spend of the prepared unlock does with the value
    enum SpendKind {
        Transfer,
        Mint,
        Burn
    }

    struct LockDetail {
        address from;
        uint256 amount;
        INotoHooks.UnlockRecipient[] recipients;
        SpendKind spendKind;
    }

    function getLock(bytes32 lockId) public view returns (LockDetail memory) {
        return _locks[lockId];
    }

    function ownerOf(bytes32 lockId) public view returns (address) {
        return _locks[lockId].from;
    }

    function onLock(bytes32 lockId, address from, uint256 amount) public {
        LockDetail storage lock = _locks[lockId];
        lock.from = from;
        lock.amount = amount;
        lockedBalance[from] += amount;
    }

    function onUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) public {
        LockDetail storage lock = _locks[lockId];
        for (uint256 i = 0; i < recipients.length; i++) {
            lock.amount -= recipients[i].amount;
            lockedBalance[lock.from] -= recipients[i].amount;
        }

        _releasePending(lock);
        if (lock.amount == 0) {
            delete _locks[lockId];
        }
    }

    function onPrepareUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) public {
        _prepareUnlock(lockId, SpendKind.Transfer, recipients);
    }

    function onPrepareMintUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) public {
        _prepareUnlock(lockId, SpendKind.Mint, recipients);
    }

    function onPrepareBurnUnlock(bytes32 lockId) public {
        _prepareUnlock(
            lockId,
            SpendKind.Burn,
            new INotoHooks.UnlockRecipient[](0)
        );
    }

    function _prepareUnlock(
        bytes32 lockId,
        SpendKind spendKind,
        INotoHooks.UnlockRecipient[] memory recipients
    ) internal {
        LockDetail storage lock = _locks[lockId];
        lock.spendKind = spendKind;
        _releasePending(lock);

        for (uint256 i = 0; i < recipients.length; i++) {
            pendingBalance[recipients[i].to] += recipients[i].amount;
            lock.recipients.push(recipients[i]);
        }
    }

    function handleDelegateUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) public {
        LockDetail storage lock = _locks[lockId];
        _releasePending(lock);
        if (lock.spendKind == SpendKind.Mint) {
            // Nothing was locked: a spend mints to the recipients and a cancel has none
            delete _locks[lockId];
        } else if (isBurnSpend(lockId, recipients)) {
            lockedBalance[lock.from] -= lock.amount;
            delete _locks[lockId];
        } else {
            onUnlock(lockId, recipients);
        }
    }

    function _releasePending(LockDetail storage lock) internal {
        for (uint256 i = 0; i < lock.recipients.length; i++) {
            pendingBalance[lock.recipients[i].to] -= lock.recipients[i].amount;
        }
        delete lock.recipients;
    }

    // A burn spend has no recipients, and a burn cancel returns the whole amount to the owner
    function isBurnSpend(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) public view returns (bool) {
        return
            _locks[lockId].spendKind == SpendKind.Burn &&
            recipients.length == 0;
    }
}
