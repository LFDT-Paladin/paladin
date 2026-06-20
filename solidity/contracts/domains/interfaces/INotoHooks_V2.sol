// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {INotoHooks_V1} from "./INotoHooks_V1.sol";

/**
 * @dev V2 of the Noto hooks interface. Additive over INotoHooks_V1: it inherits all V1 hooks
 *      (including "onUnlock", used for unprepared/direct unlocks, and "handleDelegateUnlock")
 *      and adds two hooks for notary-triggered execution of a PREPARED lock's prearranged
 *      outcome: "onSpendLock" and "onCancelLock".
 *
 *      A "prepared" lock is one created via prepareUnlock/createTransferLock/createMintLock/
 *      createBurnLock, whose spend and cancel outcomes are fixed. Previously these could only
 *      be executed by a delegate submitting directly to the base ledger (reconciled afterwards
 *      via handleDelegateUnlock). These hooks fire when the notary executes the prearranged
 *      outcome on the owner's behalf.
 */
interface INotoHooks_V2 is INotoHooks_V1 {
    /**
     * @dev Called when the notary executes the prearranged spend of a PREPARED lock.
     *      Unlike onUnlock (unprepared), implementations must clear any pending balance that
     *      was recorded at prepare time before finalizing the transfer to the recipients.
     */
    function onSpendLock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external;

    /**
     * @dev Called when the notary executes the prearranged cancel of a PREPARED lock, returning
     *      the full locked balance to the lock owner. Has no recipients. Implementations must
     *      clear any pending balance recorded at prepare time.
     */
    function onCancelLock(
        address sender,
        bytes32 lockId,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external;
}
