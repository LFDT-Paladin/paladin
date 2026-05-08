// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.27;

import {ZetoTokenFactoryUpgradeable} from "zeto-contracts-0.5.0/contracts/factory_upgradeable.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

/// @title ZetoFactoryV1 — Paladin wrapper for Zeto token deployment (factoryVersion 1).
/// @notice The `data` argument is forwarded verbatim on `PaladinRegisterSmartContract_V0`. The Paladin Zeto domain plugin
/// encodes it as either legacy ABI-only bytes (v0) or `ZetoDomainConfigID_V1 || abi.encode(v1 tuple)` — see
/// `domains/zeto/pkg/types/domain_config_codec.go` in the Paladin repo. No Solidity change is required to carry v1 metadata.
contract ZetoFactoryV1 is
    ZetoTokenFactoryUpgradeable,
    IPaladinContractRegistry_V0
{
    function deploy(
        bytes32 transactionId,
        string memory tokenName,
        string memory name,
        string memory symbol,
        address initialOwner,
        bytes memory data,
        bool isNonFungible
    ) external {
        address instance;

        if (isNonFungible) {
            // deploy non-fungible token
            instance = deployZetoNonFungibleToken(
                name,
                symbol,
                tokenName,
                initialOwner
            );
        } else {
            // deploy fungible token
            instance = deployZetoFungibleToken(
                name,
                symbol,
                tokenName,
                initialOwner
            );
        }

        emit PaladinRegisterSmartContract_V0(transactionId, instance, data);
    }

    function deploy(
        bytes32 transactionId,
        string memory tokenName,
        string memory name,
        string memory symbol,
        address initialOwner,
        bytes memory data
    ) external {
        // default deploy is fungible token
        this.deploy(
            transactionId,
            tokenName,
            name,
            symbol,
            initialOwner,
            data,
            false
        );
    }
}
