// Copyright © 2024 Kaleido, Inc.
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
pragma solidity ^0.8.27;

import {Initializable} from "@openzeppelin/contracts-upgradeable/proxy/utils/Initializable.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {IZetoInitializable} from "@lfdecentralizedtrust/zeto-contracts/contracts/lib/interfaces/izeto_initializable.sol";

contract ZetoTokenFactory is Initializable, OwnableUpgradeable, UUPSUpgradeable {
    // all the addresses needed by the factory to
    // clone a Zeto token and initialize it. The
    // "implementation" is used to clone the token,
    // the rest of the addresses are used to initialize
    struct ImplementationInfo {
        address implementation;
        IZetoInitializable.VerifiersInfo verifiers;
    }

    /// @custom:storage-location erc7201:zeto.storage.ZetoTokenFactory
    struct ZetoTokenFactoryStorage {
        mapping(string => ImplementationInfo) implementations;
    }

    // keccak256(abi.encode(uint256(keccak256("zeto.storage.ZetoTokenFactory")) - 1)) & ~bytes32(uint256(0xff))
    bytes32 private constant ZETO_TOKEN_FACTORY_STORAGE_LOCATION =
        0xea00970f7a43a8afae26fee4308a7f177a9692a756b610fca8a1c058954e4800;

    event ZetoTokenDeployed(address indexed zetoToken);

    function _getZetoTokenFactoryStorage()
        private
        pure
        returns (ZetoTokenFactoryStorage storage $)
    {
        assembly {
            $.slot := ZETO_TOKEN_FACTORY_STORAGE_LOCATION
        }
    }

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize() public virtual initializer {
        __Ownable_init(_msgSender());
        __UUPSUpgradeable_init();
    }

    function registerImplementation(
        string memory name,
        ImplementationInfo memory implementation
    ) public onlyOwner {
        require(
            implementation.implementation != address(0),
            "Factory: implementation address is required"
        );
        require(
            implementation.verifiers.verifier != address(0),
            "Factory: verifier address is required"
        );
        // the depositVerifier and withdrawVerifier are optional
        // for the non-fungible token implementations
        ZetoTokenFactoryStorage storage $ = _getZetoTokenFactoryStorage();
        $.implementations[name] = implementation;
    }

    function deployZetoFungibleToken(
        string memory name,
        string memory symbol,
        string memory tokenImplementation,
        address initialOwner
    ) public returns (address) {
        ZetoTokenFactoryStorage storage $ = _getZetoTokenFactoryStorage();
        ImplementationInfo memory args = $.implementations[tokenImplementation];
        require(
            args.implementation != address(0),
            "Factory: failed to find implementation"
        );
        // check that the registered implementation is for a fungible token
        // and has the required verifier addresses
        require(
            args.verifiers.depositVerifier != address(0),
            "Factory: depositVerifier address is required"
        );
        require(
            args.verifiers.withdrawVerifier != address(0),
            "Factory: withdrawVerifier address is required"
        );
        require(
            args.verifiers.batchVerifier != address(0),
            "Factory: batchVerifier address is required"
        );
        require(
            args.verifiers.batchWithdrawVerifier != address(0),
            "Factory: batchWithdrawVerifier address is required"
        );
        address instance = Clones.clone(args.implementation);
        require(
            instance != address(0),
            "Factory: failed to clone implementation"
        );
        (IZetoInitializable(instance)).initialize(
            name,
            symbol,
            initialOwner,
            args.verifiers
        );
        emit ZetoTokenDeployed(instance);
        return instance;
    }

    function deployZetoNonFungibleToken(
        string memory name,
        string memory symbol,
        string memory tokenImplementation,
        address initialOwner
    ) public returns (address) {
        ZetoTokenFactoryStorage storage $ = _getZetoTokenFactoryStorage();
        ImplementationInfo memory args = $.implementations[tokenImplementation];
        require(
            args.implementation != address(0),
            "Factory: failed to find implementation"
        );
        address instance = Clones.clone(args.implementation);
        require(
            instance != address(0),
            "Factory: failed to clone implementation"
        );
        (IZetoInitializable(instance)).initialize(
            name,
            symbol,
            initialOwner,
            args.verifiers
        );
        emit ZetoTokenDeployed(instance);
        return instance;
    }

    function _authorizeUpgrade(address) internal override onlyOwner {}
}
