// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {AccessControlUpgradeable} from "@openzeppelin/contracts-upgradeable/access/AccessControlUpgradeable.sol";
import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {ILockSettlerFactory} from "./interfaces/ILockSettlerFactory.sol";
import {LockSettler} from "./LockSettler.sol";

contract LockSettlerFactory is
    ILockSettlerFactory,
    Initializable,
    UUPSUpgradeable,
    AccessControlUpgradeable
{
    address public logic;

    error InvalidZeroAddress();

    event LockSettlerDeployed(address addr);

    event LockSettlerLogicSet(address newLogic);

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize() public initializer {
        __UUPSUpgradeable_init();
        __AccessControl_init();
        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
        logic = address(new LockSettler());
    }

    /**
     * @dev Create a new LockSettler instance by cloning the logic contract.
     * @param locks The locks to spend or cancel atomically.
     */
    function create(LockSettler.LockEntry[] calldata locks) public {
        address instance = Clones.clone(logic);
        LockSettler(instance).initialize(locks);
        emit LockSettlerDeployed(instance);
    }

    function setLogic(address newLogic) public onlyRole(DEFAULT_ADMIN_ROLE) {
        require(newLogic != address(0), InvalidZeroAddress());
        logic = newLogic;

        emit LockSettlerLogicSet(newLogic);
    }

    function _authorizeUpgrade(
        address newImplementation
    ) internal override onlyRole(DEFAULT_ADMIN_ROLE) {}
}
