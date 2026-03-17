// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

/**
 * @title INotoErrors_V0
 * @dev Definitions for common errors used in Noto implementations.
 */
interface INotoErrors_V0 {
    error NotoInvalidInput(bytes32 id);

    error NotoInvalidOutput(bytes32 id);

    error NotoNotNotary(address sender);

    error NotoInvalidDelegate(bytes32 txhash, address delegate, address sender); // V0 only

    error NotoInvalidUnlockHash(bytes32 expected, bytes32 actual);

    error NotoAlreadyPrepared(bytes32 lockId);

    error NotoDuplicateTransaction(bytes32 txId);
}
