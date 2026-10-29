// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {NotoTrackerERC20Base} from "./NotoTrackerERC20Base.sol";

/**
 * @title NotoTrackerERC20
 * @dev Example Noto hooks which track all Noto token movements on a private ERC20.
 */
contract NotoTrackerERC20 is NotoTrackerERC20Base {
    function initialize(
        string memory name,
        string memory symbol,
        address notary
    ) external initializer {
        __NotoTrackerERC20_init(name, symbol, notary);
    }
}
