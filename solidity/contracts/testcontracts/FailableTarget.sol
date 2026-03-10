// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

interface IFailableTarget {
    function check() external view;
}

contract FailableTarget is IFailableTarget {
    bool public shouldFail;

    function setFail(bool _shouldFail) external {
        shouldFail = _shouldFail;
    }

    function check() external view override {
        require(!shouldFail, "Configured to fail");
    }
}
