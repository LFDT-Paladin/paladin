// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.27;

import {IZetoInitializable as IZetoInitializableV022} from "zeto-contracts-0.2.2/contracts/lib/interfaces/izeto_initializable.sol";
import {IZetoInitializable as IZetoInitializableV051} from "zeto-contracts-0.5.1/contracts/lib/interfaces/IZetoInitializable.sol";

/// Test-only clone targets for ZetoFactory_V0. Each one implements the real `IZetoInitializable` of its upstream
/// zeto-contracts release — imported from the package itself, not hand-copied — so that ZetoFactory_V0.ts proves the
/// single 0.5.1-based factory can clone and initialize implementations from either generation.
abstract contract RecordingZetoToken {
    string public name;
    string public symbol;
    address public initialOwner;
    address public verifier;
    bool public initialized;

    function _record(
        string memory _name,
        string memory _symbol,
        address _initialOwner,
        address _verifier
    ) internal {
        require(!initialized, "already initialized");
        initialized = true;
        name = _name;
        symbol = _symbol;
        initialOwner = _initialOwner;
        verifier = _verifier;
    }
}

/// Implements the zeto-contracts 0.2.2 interface, whose VerifiersInfo fields are plain `address`.
contract MockZetoTokenV022 is RecordingZetoToken, IZetoInitializableV022 {
    function initialize(
        string memory _name,
        string memory _symbol,
        address _initialOwner,
        VerifiersInfo memory verifiersInfo
    ) external override {
        _record(_name, _symbol, _initialOwner, verifiersInfo.verifier);
    }
}

/// Implements the zeto-contracts 0.5.1 interface, whose VerifiersInfo fields are `IGroth16Verifier`.
contract MockZetoTokenV051 is RecordingZetoToken, IZetoInitializableV051 {
    function initialize(
        string calldata _name,
        string calldata _symbol,
        address _initialOwner,
        VerifiersInfo calldata verifiersInfo
    ) external override {
        _record(
            _name,
            _symbol,
            _initialOwner,
            address(verifiersInfo.verifier)
        );
    }
}
