// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title IZetoFungibleV1
 * @dev ABI for the Zeto domain transaction interface, specifically implemented in Go.
 *      Note: This interface is not meant for direct implementation in smart contracts.
 */
interface IZetoFungibleV1 {
    struct TransferParam {
        string to;
        uint256 amount;
        bytes data;
    }

    function mint(TransferParam[] memory mints) external;

    function transfer(TransferParam[] memory transfers) external;

    /// @notice Domain transaction shape (Paladin); not the on-chain ILockableCapability.createLock entrypoint.
    /// @param from the address that creates the lock.
    /// @param recipients intended recipients for the tokens when the lock is spent. Used to construct the spendCommitment. The cancelCommitment is constructed to refund the tokens back to the from address.
    /// @param unlockData application-specific data that will be used in the spend/cancel operations.
    /// @param data application-specific data that will be used in the create operation.
    function createLock(
        string calldata from,
        TransferParam[] calldata recipients,
        bytes calldata unlockData,
        bytes calldata data
    ) external;

    /// @notice Domain transaction shape (Paladin); not the on-chain ILockableCapability.spendLock entrypoint.
    /// @param lockId the unique identifier for the lock. This is used to retrieve the lock info from the local state storage.
    /// @param from the address that spends the lock.
    /// @param data application-specific data that will be used in the spend operation.
    function spendLock(
        bytes32 lockId,
        string calldata from,
        bytes calldata data
    ) external;

    /// @notice Domain transaction shape (Paladin); not the on-chain ILockableCapability.cancelLock entrypoint.
    /// @param lockId the unique identifier for the lock.
    /// @param from the address that cancels the lock (must be the current lock spender on-chain).
    /// @param data application-specific data that will be used in the cancel operation.
    function cancelLock(
        bytes32 lockId,
        string calldata from,
        bytes calldata data
    ) external;

    function deposit(uint256 amount) external;

    function withdraw(uint256 amount) external;

    function setERC20(address erc20) external;

    function balanceOf(
        string memory account
    )
        external
        view
        returns (uint256 totalStates, uint256 totalBalance, bool overflow);
}
