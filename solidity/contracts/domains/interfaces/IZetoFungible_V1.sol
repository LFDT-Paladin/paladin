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

    function createLock(uint256 amount, bytes calldata data) external;
    function spendLock(
        bytes32 lockId,
        TransferParam[] calldata recipients,
        bytes calldata data
    ) external;

    function deposit(uint256 amount) external;
    function withdraw(uint256 amount) external;
    function setERC20(address erc20) external;
    function balanceOf(string memory account) external view returns (uint256 totalStates, uint256 totalBalance, bool overflow);
}
