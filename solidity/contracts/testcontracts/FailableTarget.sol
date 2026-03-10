// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

interface IFailableTarget {
    function check() external view;
}

contract FailableTarget is IFailableTarget {
    uint256 public failEvery;

    constructor(uint256 _failEvery) {
        failEvery = _failEvery;
    }

    function check() external view override {
        if (failEvery > 0 && block.number % failEvery == 0) {
            revert("Configured to fail");
        }
    }
}
