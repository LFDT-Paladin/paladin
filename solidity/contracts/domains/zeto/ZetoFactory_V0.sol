// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.27;

import {ZetoTokenFactoryUpgradeable} from "zeto-contracts-0.5.1/contracts/factory_upgradeable.sol";
import {IPaladinContractRegistry_V0} from "../interfaces/IPaladinContractRegistry.sol";

/// @title ZetoFactory_V0 — the single Paladin wrapper for Zeto token deployment.
/// @notice One contract serves every `factoryVersion` the Zeto domain plugin supports (0 and 1). It is built against
/// zeto-contracts 0.5.1, which is a drop-in superset of 0.2.2 for factory purposes: the two upstream
/// `ZetoTokenFactoryUpgradeable` contracts differ only by `address(...)` casts introduced when `VerifiersInfo` fields
/// were retyped from `address` to `IGroth16Verifier` (same `address` wire type), so `registerImplementation`,
/// `deployZetoFungibleToken` and `deployZetoNonFungibleToken` are selector- and behaviour-identical across both.
///
/// Compatibility guarantees this contract must preserve:
///  1. NEW deployments of either upstream generation. The factory only stores implementation addresses and clones
///     them, calling `IZetoInitializable.initialize(string,string,address,(address x9))` — an unchanged selector from
///     0.2.2 through 0.5.1 — so implementations compiled from either release can be registered and deployed here.
///  2. EXISTING deployments made by the legacy, NON-upgradeable wrapper — the pre-2026 version of this contract, which
///     extended `ZetoTokenFactory` from `zeto-contracts/contracts/factory.sol`. The domain plugin talks to an already-deployed
///     factory only through `deploy(bytes32,string,string,string,address,bytes,bool)` (0x653bf99c) and its 6-argument
///     overload (0x05c98c83). Both selectors are identical here, so this ABI encodes valid calls against those legacy
///     addresses even though they expose no `initialize` / `upgradeToAndCall`. See TestZetoFactoryLegacyDeploySelectors.
///
/// The `data` argument is forwarded verbatim on `PaladinRegisterSmartContract_V0`. The Paladin Zeto domain plugin
/// encodes it as either legacy ABI-only bytes (v0) or `ZetoDomainConfigID_V1 || abi.encode(v1 tuple)` — see
/// `domains/zeto/pkg/types/domain_config_codec.go` in the Paladin repo. No Solidity change is required to carry v1
/// metadata.
///
/// `initialize()` and the `_disableInitializers()` constructor are inherited from `ZetoTokenFactoryUpgradeable`; do not
/// re-declare them here. An override that wraps the base `initializer` in a second `initializer` modifier only passes
/// while `address(this).code.length == 0`, which restricts deployment to the "initialize inside the ERC1967Proxy
/// constructor" flow and reverts on a post-deployment `initialize()` call.
contract ZetoFactory_V0 is
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
