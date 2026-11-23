// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

/// @title Calls a precompile and returns its raw output
contract PrecompileCaller {
    function callPrecompile(
        address precompile,
        bytes calldata input
    ) external view returns (bool success, bytes memory output) {
        (success, output) = precompile.staticcall(input);
    }
}
