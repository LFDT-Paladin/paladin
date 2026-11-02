// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

import {Clones} from "@openzeppelin/contracts/proxy/Clones.sol";
import {SimpleStorage} from './SimpleStorage.sol';

/// @title Embeds an EIP-1167 clone of a simple storage, and invokes it to do the work
contract SimpleStorageCloned {

  SimpleStorage public ss;

  constructor(address implementation) {
    ss = SimpleStorage(Clones.clone(implementation));
  }

  function set(uint _x) public {
    ss.set(_x);
  }

  function get() view public returns (uint x) {
    return ss.get();
  }
}
