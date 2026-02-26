// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IConfidentialToken} from "../interfaces/IConfidentialToken.sol";
import {ILockableCapability} from "../interfaces/ILockableCapability.sol";

/**
 * @title INoto
 * @dev All implementations of Noto must conform to this interface.
 */
interface INoto is IConfidentialToken, ILockableCapability {

    // The structure definition for a Noto create lock operation, which includes:
    // - The input states to be consumed in this operation. e.g. the value to be locked
    // - The output states to be generated in this operation. e.g. if there is any change to return
    // - The contents that were stored in the lock. e.g. the locked value states
    struct NotoCreateLockOperation {
        bytes32 txId;
        bytes32 newLockState; 
        bytes32[] inputs; // spent in the transaction
        bytes32[] outputs; // created outside the lock by the transaction
        bytes32[] contents; // created inside of the lock - this array ABI encoded is the lock contents (can be empty for mint-locks)
        bytes proof; // recorded signature for the lock operation
    }

    // The structure definition for a Noto update lock operation, which includes:
    // - The input states to be consumed in this operation. e.g. the old lock record
    // - The output states to be generated in this operation. e.g. the new lock record
    struct NotoUpdateLockOperation {
        bytes32 txId;
        bytes32 oldLockState;
        bytes32 newLockState;
        bytes proof; // recorded signature for the lock operation
    }

    // The structure definition for a Noto unlock operation, which can be hashed
    // in order to construct a spendHash or a cancelHash.
    struct NotoUnlockOperation {
        bytes32 txId;
        bytes32[] inputs; // exactly per the spendHash/cancelHash, if set in the lock state
        bytes32[] outputs; // exactly per the spendHash/cancelHash, if set in the lock state
        bytes data; // exactly per the spendHash/cancelHash, meaning this is the data from the time of the prepare
        bytes proof; // does not contribute to the hash
    }

    // The structure definition for a Noto delegate operation
    struct NotoDelegateOperation {
        bytes32 txId;
        bytes32 oldLockState;
        bytes32 newLockState;
        bytes proof;
    }

    // The structure definition for Noto options within a LockInfo
    struct NotoLockOptions {
        // A unique transaction ID that must be used to spend or cancel the lock.
        bytes32 spendTxId;
    }
    
    // The Noto event for creation of a lock contains the inputs,outputs and contents
    event NotoLockCreated(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed owner,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes32[] contents,
        bytes32 newLockState,
        bytes proof,
        bytes data
    );

    // The Noto event for updating of a lock contains lockState transition
    // Note: The contents cannot be changed in this operation, they are emitted to allow preparing of the unlock operation
    event NotoLockUpdated(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed operator,
        bytes32[] contents,
        bytes32 oldLockState,
        bytes32 newLockState,
        bytes proof,
        bytes data
    );

    // The Noto event for spending of a lock, contains the inputs, outputs, data and consumed lockState.
    // Note: the spendHash/cancelHash only cover the txId, inputs, outputs, data
    event NotoLockSpent(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed spender,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes txData,
        bytes32 oldLockState,
        bytes proof,
        bytes data
    );

    // The Noto event for cancelling a lock, contains the inputs, outputs, data and consumed lockState.
    // Note: the spendHash/cancelHash only cover the txId, inputs, outputs, data
    event NotoLockCancelled(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed spender,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes txData,
        bytes32 oldLockState,
        bytes proof,
        bytes data
    );

    // The Noto event for delegating the lock, including the txId, proof and lockState transition
    event NotoLockDelegated(
        bytes32 indexed txId,
        bytes32 indexed lockId,
        address indexed from,
        address to,
        bytes32 oldLockState,
        bytes32 newLockState,
        bytes proof,
        bytes data
    );

    function initialize(
        string memory name_,
        string memory symbol_,
        address notary
    ) external;

    function buildConfig(
        bytes calldata data
    ) external view returns (bytes memory);

    /**
     * @dev Compute the lockId for given parameters (deterministic generation).
     *      This allows callers to predict the lockId before calling createLock().
     *
     * @param createInputs The inputs that will be passed to the createLock call
     * @return lockId The computed unique identifier for the lock.
     */
    function computeLockId(
        bytes calldata createInputs
    ) external view returns (bytes32 lockId);

   /**
     * @dev Query the lockId for a locked state.
     *
     * @param id The state identifier.
     * @return lockId The lockId set when the lock was created.
     */
    function getLockId(bytes32 id) external view returns (bytes32 lockId);

}