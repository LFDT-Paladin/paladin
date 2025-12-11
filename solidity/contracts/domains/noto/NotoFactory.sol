// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {ERC1967Proxy} from "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";
import {INoto} from "../interfaces/INoto.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

// NotoFactory version: 2
contract NotoFactory is
    Initializable,
    OwnableUpgradeable,
    UUPSUpgradeable,
    IPaladinContractRegistry_V0
{
    /// @custom:storage-location erc7201:paladin.storage.NotoFactory
    struct NotoFactoryStorage {
        mapping(string => address) implementations;
    }

    // keccak256(abi.encode(uint256(keccak256("paladin.storage.NotoFactory")) - 1)) & ~bytes32(uint256(0xff))
    bytes32 private constant NOTO_FACTORY_STORAGE_LOCATION =
        0xe67f96c01195ce35b5ff7e8a6398509283372467daaa337070e1893bae2d8600;

    function _getNotoFactoryStorage()
        private
        pure
        returns (NotoFactoryStorage storage $)
    {
        assembly {
            $.slot := NOTO_FACTORY_STORAGE_LOCATION
        }
    }

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
        NotoFactoryStorage storage $ = _getNotoFactoryStorage();
        $.implementations["default"] = defaultImplementation;
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
        NotoFactoryStorage storage $ = _getNotoFactoryStorage();
        _deploy($.implementations["default"], transactionId, name, symbol, notary, data);
    }

    /**
     * Register an additional implementation of Noto.
     */
    function registerImplementation(
        string calldata name,
        address implementation
    ) public onlyOwner {
        NotoFactoryStorage storage $ = _getNotoFactoryStorage();
        $.implementations[name] = implementation;
    }

    /**
     * Query an implementation
     */
    function getImplementation(
        string calldata name
    ) public view returns (address implementation) {
        NotoFactoryStorage storage $ = _getNotoFactoryStorage();
        return $.implementations[name];
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
        NotoFactoryStorage storage $ = _getNotoFactoryStorage();
        _deploy($.implementations[implementationName], transactionId, name, symbol, notary, data);
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
