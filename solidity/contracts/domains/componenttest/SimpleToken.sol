// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

// SIMDomain is an un-optimized, simplistic test tool that is used in the unit tests of the test bed
// PLEASE REFER TO ZETO, NOTO AND PENTE FOR REAL EXAMPLES OF ACTUAL IMPLEMENTED DOMAINS
contract SimpleToken {

    event UTXOTransfer(
        bytes32 txId,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes signature
    );

    uint256 public storedAmount = 0;

    error BadNotary(address sender);
    error SimpleTokenRetryableError(bytes32 id);
    error SimpleTokenNonRetryableError(bytes32 id);
    bytes32 public constant SINGLE_FUNCTION_SELECTOR = keccak256("SimpleToken()");
    
    constructor() {
    }

    function paladinExecute_V0(bytes32 txId, bytes32 fnSelector, bytes calldata payload) public  {
        assert(fnSelector == SINGLE_FUNCTION_SELECTOR);
        (bytes32 signature, bytes32[] memory inputs, bytes32[] memory outputs) =
            abi.decode(payload, (bytes32, bytes32[], bytes32[]));
        emit UTXOTransfer(txId, inputs, outputs, abi.encodePacked(signature));
    }

    function executeNotarized(bytes32 txId, bytes32[] calldata inputs, bytes32[] calldata outputs, bytes calldata signature, uint256 errorMode) public {

        if (errorMode == 1) {
            revert SimpleTokenRetryableError(outputs.length > 0 ? outputs[0] : bytes32(0));
        }
        if (errorMode == 2) {
            revert SimpleTokenNonRetryableError(outputs.length > 0 ? outputs[0] : bytes32(0));
        }
        emit UTXOTransfer(txId, inputs, outputs, abi.encodePacked(signature));
    }

    // Version of the on-chain function that exposes the actual amount the transfer represents and enforces a fixed validation rule
    // that every amount muust be +=1 the previous amount. We use this to exercise simple on-ledger in-order delivery in the tests
    function executeNotarizedAmountExposed(bytes32 txId, bytes32[] calldata inputs, bytes32[] calldata outputs, bytes calldata signature, uint256 amount) public {
        if (amount != storedAmount + 1) {
            revert("tx arrived out of order, amount not expected");
        }
        storedAmount = amount;
        emit UTXOTransfer(txId, inputs, outputs, abi.encodePacked(signature));
    }

    function executeNotarizedHook(bytes32 txId, bytes32[] calldata inputs, bytes32[] calldata outputs, bytes calldata signature, bytes32 originTxId, uint256 errorMode) public {
        if (errorMode == 1) {
            revert SimpleTokenRetryableError(outputs.length > 0 ? outputs[0] : bytes32(0));
        }
        if (errorMode == 2) {
            revert SimpleTokenNonRetryableError(outputs.length > 0 ? outputs[0] : bytes32(0));
        }
        // Emit 2 events, one for the hook TX ID, one for the original TX ID. Note that the simple domain
        // doesn't check the inputs and outputs so we just pass them through to both. In reality the origin
        // domain wouldn't validate the inputs and outputs but we're just testing TX chaining here, not domain functionality.
        emit UTXOTransfer(txId, inputs, outputs, abi.encodePacked(signature));
        emit UTXOTransfer(originTxId, inputs, outputs, abi.encodePacked(signature));
    }

}
