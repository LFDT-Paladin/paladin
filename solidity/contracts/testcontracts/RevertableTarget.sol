// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

interface IRevertableTarget {
    function check() external pure;
    function checkNotoInvalidInput() external pure;
}

error NotoInvalidInput(bytes32 input);

contract RevertableTarget is IRevertableTarget {
    function check() external pure override {
        revert("Configured to fail");
    }

    function checkNotoInvalidInput() external pure override {
        revert NotoInvalidInput(bytes32(0));
    }
}
