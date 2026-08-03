// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.0;

import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {MessageHashUtils} from "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";

contract Recover {
    using ECDSA for bytes32;

    function verifySignature(
        bytes32 messageHash,
        bytes calldata signature,
        address expectedSigner
    ) external pure returns (bool valid, address recoveredSigner) {
        // Compute the eth_sign hash
        bytes32 ethSignedHash = MessageHashUtils.toEthSignedMessageHash(
            messageHash
        );

        // Try to recover the signer
        if (signature.length == 65) {
            recoveredSigner = ethSignedHash.recover(signature);
        }

        valid = recoveredSigner == expectedSigner;

        return (valid, recoveredSigner);
    }

} 