// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {ILockSettler} from "./ILockSettler.sol";

interface ILockSettlerFactory {
    function create(ILockSettler.LockEntry[] calldata locks) external;
}
