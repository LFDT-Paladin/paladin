// Copyright contributors to Paladin, an LFDT project
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
pragma solidity ^0.8.20;

import {ERC20Upgradeable} from "@openzeppelin/contracts-upgradeable/token/ERC20/ERC20Upgradeable.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {INotoHooks} from "../domains/interfaces/INotoHooks.sol";
import {NotoLocks} from "./NotoLocks.sol";

/**
 * @title NotoTrackerERC20Base
 * @dev Noto hooks which track all Noto token movements on a private ERC20.
 */
abstract contract NotoTrackerERC20Base is
    INotoHooks,
    ERC20Upgradeable,
    OwnableUpgradeable,
    UUPSUpgradeable,
    NotoLocks
{
    /// @custom:storage-location erc7201:paladin.storage.NotoTrackerERC20Base
    struct NotoTrackerERC20BaseStorage {
        address notary;
    }

    // keccak256(abi.encode(uint256(keccak256("paladin.storage.NotoTrackerERC20Base")) - 1)) & ~bytes32(uint256(0xff))
    bytes32 private constant NotoTrackerERC20BaseStorageLocation =
        0xe8cf9158af53736567d55f24ee80acb5145cef9a479bc6afdfcdc34704c50f00;

    function _getNotoTrackerERC20BaseStorage()
        private
        pure
        returns (NotoTrackerERC20BaseStorage storage $)
    {
        assembly {
            $.slot := NotoTrackerERC20BaseStorageLocation
        }
    }

    modifier onlyNotary(address sender) {
        require(
            sender == _getNotoTrackerERC20BaseStorage().notary,
            "Sender is not the notary"
        );
        _;
    }

    modifier onlySelf(address sender, address from) {
        require(sender == from, "Sender is not the from address");
        _;
    }

    modifier onlyLockOwner(address sender, bytes32 lockId) {
        require(
            sender == lockOwner(lockId),
            "Sender is not the lock owner"
        );
        _;
    }

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function __NotoTrackerERC20_init(
        string memory name,
        string memory symbol,
        address notary
    ) internal onlyInitializing {
        __ERC20_init(name, symbol);
        __Ownable_init(_msgSender());
        __UUPSUpgradeable_init();
        _getNotoTrackerERC20BaseStorage().notary = notary;
    }

    function transfer(
        address /* to */,
        uint256 /* value */
    ) public pure override returns (bool) {
        revert(
            "Direct transfers are disabled: balances only change through Noto hooks"
        );
    }

    function approve(
        address /* spender */,
        uint256 /* value */
    ) public pure override returns (bool) {
        revert(
            "Direct transfers are disabled: balances only change through Noto hooks"
        );
    }

    function transferFrom(
        address /* from */,
        address /* to */,
        uint256 /* value */
    ) public pure override returns (bool) {
        revert(
            "Direct transfers are disabled: balances only change through Noto hooks"
        );
    }

    function _onMint(
        address /* sender */,
        address to,
        uint256 amount,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) internal virtual onlyNotary(msg.sender) {
        _mint(to, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onTransfer(
        address /* sender */,
        address from,
        address to,
        uint256 amount,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) internal virtual onlyNotary(msg.sender) {
        _transfer(from, to, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onBurn(
        address /* sender */,
        address from,
        uint256 amount,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) internal virtual onlyNotary(msg.sender) {
        _burn(from, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onLock(
        address /* sender */,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) internal virtual onlyNotary(msg.sender) {
        NotoLocks._onLock(lockId, from, amount);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onUnlock(
        address /* sender */,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) internal virtual onlyNotary(msg.sender) {
        address from = lockOwner(lockId);
        NotoLocks._onUnlock(lockId, recipients);
        for (uint256 i = 0; i < recipients.length; i++) {
            _transfer(from, recipients[i].to, recipients[i].amount);
        }
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function _onPrepareUnlock(
        address /* sender */,
        bytes32 lockId,
        UnlockRecipient[] memory recipients,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) internal virtual onlyNotary(msg.sender) {
        NotoLocks._onPrepareUnlock(lockId, recipients);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onMint(
        address sender,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyNotary(sender) {
        _onMint(sender, to, amount, data, prepared);
    }

    function onTransfer(
        address sender,
        address from,
        address to,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlySelf(sender, from) {
        _onTransfer(sender, from, to, amount, data, prepared);
    }

    function onBurn(
        address sender,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlySelf(sender, from) {
        _onBurn(sender, from, amount, data, prepared);
    }

    function onLock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlySelf(sender, from) {
        _onLock(sender, lockId, from, amount, data, prepared);
    }

    function onPrepareMintUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) external virtual override onlyNotary(msg.sender) onlyNotary(sender) {
        NotoLocks._onPrepareMintUnlock(lockId, recipients);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onPrepareBurnUnlock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 /* amount */,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) external virtual override onlyNotary(msg.sender) onlySelf(sender, from) {
        NotoLocks._onPrepareBurnUnlock(lockId);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyLockOwner(sender, lockId) {
        _onUnlock(sender, lockId, recipients, data, prepared);
    }

    function onPrepareUnlock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlyLockOwner(sender, lockId) {
        _onPrepareUnlock(sender, lockId, recipients, data, prepared);
    }

    function onCreateTransferLock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        UnlockRecipient[] calldata recipients,
        bytes calldata data,
        PreparedTransaction calldata prepared
    ) external virtual override onlySelf(sender, from) {
        NotoLocks._onLock(lockId, from, amount);
        _onPrepareUnlock(sender, lockId, recipients, data, prepared);
    }

    function onCreateMintLock(
        address sender,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) external virtual override onlyNotary(msg.sender) onlyNotary(sender) {
        NotoLocks._onPrepareMintUnlock(lockId, recipients);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onCreateBurnLock(
        address sender,
        bytes32 lockId,
        address from,
        uint256 amount,
        bytes calldata /* data */,
        PreparedTransaction calldata prepared
    ) external virtual override onlyNotary(msg.sender) onlySelf(sender, from) {
        NotoLocks._onLock(lockId, from, amount);
        NotoLocks._onPrepareBurnUnlock(lockId);
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function onDelegateLock(
        address /* sender */,
        bytes32 /* lockId */,
        address /* delegate */,
        PreparedTransaction calldata prepared
    ) external virtual override onlyNotary(msg.sender) {
        emit PenteExternalCall(prepared.contractAddress, prepared.encodedCall);
    }

    function handleDelegateUnlock(
        address /* sender */,
        bytes32 lockId,
        UnlockRecipient[] calldata recipients,
        bytes calldata /* data */
    ) external virtual override onlyNotary(msg.sender) {
        LockDetail memory lock = _getLock(lockId);
        bool burn = _isBurnSpend(lockId, recipients);
        NotoLocks._handleDelegateUnlock(lockId, recipients);
        if (burn) {
            _burn(lock.from, lock.amount);
        }
        for (uint256 i = 0; i < recipients.length; i++) {
            if (lock.spendKind == SpendKind.Mint) {
                _mint(recipients[i].to, recipients[i].amount);
            } else {
                _transfer(lock.from, recipients[i].to, recipients[i].amount);
            }
        }
    }

    function _authorizeUpgrade(address) internal override onlyOwner {}
}
