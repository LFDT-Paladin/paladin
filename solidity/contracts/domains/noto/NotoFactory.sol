// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {ERC1967Proxy} from "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import {INoto} from "../interfaces/INoto.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

// NotoFactory version: 2
error UnknownImplementation(string name);

contract NotoFactory is
    Initializable,
    OwnableUpgradeable,
    UUPSUpgradeable,
    IPaladinContractRegistry_V0
{
    mapping(string => address) internal implementations;

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    /**
     * Initialize the factory with a default Noto implementation.
     * @param defaultImplementation Address of the deployed Noto implementation contract
     */
    function initialize(address defaultImplementation) public initializer {
        __Ownable_init(_msgSender());
        __UUPSUpgradeable_init();
        implementations["default"] = defaultImplementation;
    }

    /**
     * Deploy a default instance of Noto.
     */
    function deploy(
        bytes32 transactionId,
        string calldata name,
        string calldata symbol,
        address notary,
        bytes calldata data
    ) external {
        _deploy(
            implementations["default"],
            transactionId,
            name,
            symbol,
            notary,
            data
        );
    }

    /**
     * Register an additional implementation of Noto.
     */
    function registerImplementation(
        string calldata name,
        address implementation
    ) public onlyOwner {
        implementations[name] = implementation;
    }

    /**
     * Query an implementation
     */
    function getImplementation(
        string calldata name
    ) public view returns (address implementation) {
        return implementations[name];
    }

    /**
     * Deploy an instance of Noto by cloning a specific implementation.
     */
    function deployImplementation(
        bytes32 transactionId,
        string calldata implementationName,
        string calldata name,
        string calldata symbol,
        address notary,
        bytes calldata data
    ) external {
        address impl = implementations[implementationName];
        if (impl == address(0))
            revert UnknownImplementation(implementationName);
        _deploy(impl, transactionId, name, symbol, notary, data);
    }

    function _deploy(
        address implementation,
        bytes32 transactionId,
        string calldata name,
        string calldata symbol,
        address notary,
        bytes calldata data
    ) internal {
        address instance = address(
            new ERC1967Proxy(
                implementation,
                abi.encodeCall(INoto.initialize, (name, symbol, notary))
            )
        );

        emit PaladinRegisterSmartContract_V0(
            transactionId,
            instance,
            INoto(instance).buildConfig(data)
        );
    }

    function _authorizeUpgrade(address) internal override onlyOwner {}
}
