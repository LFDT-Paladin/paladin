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
    address internal implementation;

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize() public initializer {
        __Ownable_init(_msgSender());
        __UUPSUpgradeable_init();
        implementation = address(new PentePrivacyGroup());
    }

    function newPrivacyGroup(
        bytes32 transactionId,
        bytes memory data
    ) external {
        address instance = address(
            new ERC1967Proxy(
                implementation,
                abi.encodeCall(PentePrivacyGroup.initialize, (data))
            )
        );

        emit PaladinRegisterSmartContract_V0(transactionId, instance, data);
    }

    function _authorizeUpgrade(address) internal override onlyOwner {}
}
