// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {IConfidentialToken} from "../interfaces/IConfidentialToken.sol";
import {ILockableCapability} from "../interfaces/ILockableCapability.sol";

/**
 * @title INoto
 * @dev All implementations of Noto must conform to this interface.
 */
interface INoto is IConfidentialToken, ILockableCapability {

    // The Noto event for creation of a lock contains the inputs,outputs an lockedOutputs
    event NotoLockCreated(
        bytes32 txId,
        address indexed operator,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes32[] lockedOutputs,
        bytes proof,
        bytes data
    );

    // The Noto event for creation of a lock contains the inputs,outputs an lockedOutputs
    event NotoLockUpdated(
        bytes32 txId,
        address indexed operator,
        bytes32[] inputs,
        bytes32[] outputs,
        bytes proof,
        bytes data
    );

    // The structure definition for a Noto lock operation, which defines
    // the inputs that will be turned into lockedOutputs.
    // The same input is used for both createLock and updateLock, however the updateLock
    // must not output lockedOutputs (as the contents of the lock are not mutable).
    // A UTXO state is used to track the lock itself, thus the need to have inputs/outputs
    // on the update operation.
    struct NotoLockOperation {
        bytes32 txId;
        bytes32[] inputs; // spent in the transaction
        bytes32[] outputs; // created outside the lock by the transaction
        bytes32[] lockedOutputs; // created inside of the lock - this array ABI encoded is the lock contents (can be empty for mint-locks)
        bytes proof; // recorded signature for the lock operation
    }

    // The structure definition for a Noto unlock operation, which can be hashed
    // in order to construct a spendHash or a cancelHash
    struct NotoUnlockOperation {
        bytes32 txId;
        bytes32[] inputs;
        bytes32[] outputs;
        bytes data; // this is the inner-data of the prepared transaction (not the unlock)
    }

    // The structure definition for a Noto delegate operation
    struct NotoDelegateOperation {
        bytes32 txId;
    }

    // The structure definition for Noto options within a LockInfo
    struct NotoLockOptions {
        // A unique transaction ID that must be used to spend or cancel the lock.
        bytes32 spendTxId;
    }

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