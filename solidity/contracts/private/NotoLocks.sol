// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {INotoHooks} from "../domains/interfaces/INotoHooks.sol";

/**
 * Helpers for tracking locked amounts from Noto hooks contracts.
 */
abstract contract NotoLocks {
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

    /// @custom:storage-location erc7201:paladin.storage.NotoLocks
    struct NotoLocksStorage {
        // Details on all currently active locks and their possible unlocks
        mapping(bytes32 => LockDetail) locks;
        // Balances locked by address (still logically owned by that address)
        mapping(address => uint256) lockedBalance;
        // Pending balances from prepared unlocks (not yet owned, but approved to be owned when unlocked)
        mapping(address => uint256) pendingBalance;
    }

    // keccak256(abi.encode(uint256(keccak256("paladin.storage.NotoLocks")) - 1)) & ~bytes32(uint256(0xff))
    bytes32 private constant NotoLocksStorageLocation =
        0x62561cd9d94a7be9f06c0f75bf340f8dc936c64a38b9be102c1a636477d67300;

    function _getNotoLocksStorage()
        private
        pure
        returns (NotoLocksStorage storage $)
    {
        assembly {
            $.slot := NotoLocksStorageLocation
        }
    }

    function lockOwner(bytes32 lockId) public view returns (address) {
        return _getNotoLocksStorage().locks[lockId].from;
    }

    function lockedBalance(address account) public view returns (uint256) {
        return _getNotoLocksStorage().lockedBalance[account];
    }

    function pendingBalance(address account) public view returns (uint256) {
        return _getNotoLocksStorage().pendingBalance[account];
    }

    function _getLock(
        bytes32 lockId
    ) internal view returns (LockDetail memory) {
        return _getNotoLocksStorage().locks[lockId];
    }

    function _onLock(bytes32 lockId, address from, uint256 amount) internal {
        NotoLocksStorage storage $ = _getNotoLocksStorage();
        LockDetail storage lock = $.locks[lockId];
        lock.from = from;
        lock.amount = amount;
        $.lockedBalance[from] += amount;
    }

    function _onUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) internal {
        NotoLocksStorage storage $ = _getNotoLocksStorage();
        LockDetail storage lock = $.locks[lockId];
        for (uint256 i = 0; i < recipients.length; i++) {
            lock.amount -= recipients[i].amount;
            $.lockedBalance[lock.from] -= recipients[i].amount;
        }

        _releasePending(lock);
        if (lock.amount == 0) {
            delete $.locks[lockId];
        }
    }

    function _onPrepareUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] memory recipients
    ) internal {
        _prepareUnlock(lockId, SpendKind.Transfer, recipients);
    }

    function _onPrepareMintUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] memory recipients
    ) internal {
        _prepareUnlock(lockId, SpendKind.Mint, recipients);
    }

    function _onPrepareBurnUnlock(bytes32 lockId) internal {
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
    ) private {
        NotoLocksStorage storage $ = _getNotoLocksStorage();
        LockDetail storage lock = $.locks[lockId];
        lock.spendKind = spendKind;
        _releasePending(lock);

        for (uint256 i = 0; i < recipients.length; i++) {
            $.pendingBalance[recipients[i].to] += recipients[i].amount;
            lock.recipients.push(recipients[i]);
        }
    }

    function _handleDelegateUnlock(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) internal {
        NotoLocksStorage storage $ = _getNotoLocksStorage();
        LockDetail storage lock = $.locks[lockId];
        _releasePending(lock);
        if (lock.spendKind == SpendKind.Mint) {
            // Nothing was locked: a spend mints to the recipients and a cancel has none
            delete $.locks[lockId];
        } else if (_isBurnSpend(lockId, recipients)) {
            $.lockedBalance[lock.from] -= lock.amount;
            delete $.locks[lockId];
        } else {
            _onUnlock(lockId, recipients);
        }
    }

    function _releasePending(LockDetail storage lock) private {
        NotoLocksStorage storage $ = _getNotoLocksStorage();
        for (uint256 i = 0; i < lock.recipients.length; i++) {
            $.pendingBalance[lock.recipients[i].to] -= lock
                .recipients[i]
                .amount;
        }
        delete lock.recipients;
    }

    // A burn spend has no recipients, and a burn cancel returns the whole amount to the owner
    function _isBurnSpend(
        bytes32 lockId,
        INotoHooks.UnlockRecipient[] calldata recipients
    ) internal view returns (bool) {
        return
            _getNotoLocksStorage().locks[lockId].spendKind == SpendKind.Burn &&
            recipients.length == 0;
    }
}
