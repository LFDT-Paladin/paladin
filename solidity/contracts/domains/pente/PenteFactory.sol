// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {ERC1967Proxy} from "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import {PentePrivacyGroup} from "./PentePrivacyGroup.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

contract PenteFactory is
    Initializable,
    OwnableUpgradeable,
    UUPSUpgradeable,
    IPaladinContractRegistry_V0
{
    /// @custom:storage-location erc7201:paladin.storage.PenteFactory
    struct PenteFactoryStorage {
        address implementation;
    }

    // keccak256(abi.encode(uint256(keccak256("paladin.storage.PenteFactory")) - 1)) & ~bytes32(uint256(0xff))
    bytes32 private constant PENTE_FACTORY_STORAGE_LOCATION =
        0xadb3aa9174002b5e885df4cdfe8d596d5a9e86def387597bd1db82a714a08000;

    function _getPenteFactoryStorage()
        private
        pure
        returns (PenteFactoryStorage storage $)
    {
        assembly {
            $.slot := PENTE_FACTORY_STORAGE_LOCATION
        }
    }

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize() public initializer {
        __Ownable_init(_msgSender());
        __UUPSUpgradeable_init();
        PenteFactoryStorage storage $ = _getPenteFactoryStorage();
        $.implementation = address(new PentePrivacyGroup());
    }

    function newPrivacyGroup(
        bytes32 transactionId,
        bytes memory data
    ) external {
        PenteFactoryStorage storage $ = _getPenteFactoryStorage();
        address instance = address(
            new ERC1967Proxy(
                $.implementation,
                abi.encodeCall(PentePrivacyGroup.initialize, (data))
            )
        );

        emit PaladinRegisterSmartContract_V0(transactionId, instance, data);
    }

    function _authorizeUpgrade(address) internal override onlyOwner {}
}
