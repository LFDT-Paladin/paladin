// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {EIP712Upgradeable} from "@openzeppelin/contracts-upgradeable/utils/cryptography/EIP712Upgradeable.sol";
import {UUPSUpgradeable} from "@openzeppelin/contracts-upgradeable/proxy/utils/UUPSUpgradeable.sol";
import {INoto} from "../interfaces/INoto.sol";
import {INotoErrors} from "../interfaces/INotoErrors.sol";

/**
 * @title Noto
 * @author Kaleido, Inc.
 * @dev An implementation of a Confidential UTXO (C-UTXO) pattern, with participant
 *      confidentiality and anonymity based on notary submission and validation of
 *      transactions.
 *
 *      The EVM ledger provides double-spend protection on the transaction outputs,
 *      and provides a deterministic linkage (a DAG) of inputs and outputs.
 *
 *      The notary must authorize every transaction by either:
 *        1. Submitting the transaction directly
 *        2. Creating a "lock" on a proposed transaction and then delegating it
 *           to be spent (executed) by another EVM address
 */
contract Noto is EIP712Upgradeable, UUPSUpgradeable, INoto, INotoErrors {
    struct NotoConfig_V1 {
        string name;
        string symbol;
        uint8 decimals;
        address notary;
        uint64 variant;
        bytes data;
    }

    // Extended from ILockableCapability.LockInfo with content/options
    struct NotoLockInfo {
        address owner;
        address spender;
        bytes32 spendCommitment;
        bytes32 cancelCommitment;
        bytes content;
        NotoLockOptions options;
    }

    // Config follows the convention of a 4 byte type selector, followed by ABI encoded bytes
    bytes4 public constant NotoConfigID_V1 = 0x00020000;

    uint64 public constant NotoVariant = 0x0002;

    bytes32 private constant UNLOCK_TYPEHASH =
        keccak256(
            "Unlock(bytes32 txId,bytes32[] lockedInputs,bytes32[] outputs,bytes data)"
        );

    string private _name;
    string private _symbol;
    address public notary;

    mapping(bytes32 => bool) private _unspent;
    mapping(bytes32 => bytes32) private _locked; // state ID => lock ID
    mapping(bytes32 => NotoLockInfo) private _locks; // lock ID => lock details
    mapping(bytes32 => bool) private _txIds; // track used transaction IDs
    mapping(bytes32 => bytes32) private _lockStates; // track the current lockState ID for any active lock
    mapping(bytes32 => bytes32) private _lockTxIds; // tx ID => lock ID (for prepared transactions)

    function requireNotary(address addr) internal view {
        if (addr != notary) {
            revert NotoNotNotary(addr);
        }
    }

    function requireSpender(bytes32 lockId, address addr) internal view {
        address spender = _locks[lockId].spender;
        if (addr != spender) {
            revert LockUnauthorized(lockId, spender, addr);
        }
    }

    function useTxId(bytes32 txId) internal {
        if (_txIds[txId]) {
            revert NotoDuplicateTransaction(txId);
        }
        _txIds[txId] = true;
    }

    modifier onlyNotary() {
        requireNotary(msg.sender);
        _;
    }

    modifier onlySpender(bytes32 lockId) {
        requireSpender(lockId, msg.sender);
        _;
    }

    modifier onlyNotaryOrSpender(bytes32 lockId) {
        NotoLockInfo storage lock = _locks[lockId];
        bool isDelegated = lock.spender != lock.owner;

        if (isDelegated) {
            // Delegated locks can only be used by the spender
            requireSpender(lockId, msg.sender);
        } else {
            // Undelegated locks can only be used by the notary
            requireNotary(msg.sender);
        }
        _;
    }

    modifier txIdNotUsed(bytes32 txId) {
        useTxId(txId);
        _;
    }

    modifier lockActive(bytes32 lockId) {
        if (_locks[lockId].owner == address(0)) {
            revert LockNotActive(lockId);
        }
        _;
    }

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(
        string memory name_,
        string memory symbol_,
        address notary_
    ) public virtual initializer {
        __EIP712_init("noto", "0.0.1");
        _name = name_;
        _symbol = symbol_;
        notary = notary_;
    }

    function buildConfig(
        bytes calldata data
    ) external view returns (bytes memory) {
        return
            _encodeConfig(
                NotoConfig_V1({
                    name: _name,
                    symbol: _symbol,
                    decimals: decimals(),
                    notary: notary,
                    variant: NotoVariant,
                    data: data
                })
            );
    }

    function _encodeConfig(
        NotoConfig_V1 memory config
    ) internal pure returns (bytes memory) {
        bytes memory configOut = abi.encode(
            config.name,
            config.symbol,
            config.decimals,
            config.notary,
            config.variant,
            config.data
        );
        return bytes.concat(NotoConfigID_V1, configOut);
    }

    function _authorizeUpgrade(address) internal override onlyNotary {}

    /**
     * @dev Returns the name of the token.
     */
    function name() external view returns (string memory) {
        return _name;
    }

    /**
     * @dev Returns the symbol of the token.
     */
    function symbol() external view returns (string memory) {
        return _symbol;
    }

    /**
     * @dev Returns the number of decimals places of the token.
     */
    function decimals() public pure returns (uint8) {
        return 4;
    }

    /**
     * @dev Query whether a state is currently in the unspent list.
     * @param id The state identifier.
     * @return unspent True or false depending on whether the identifier is in the unspent map.
     */
    function isUnspent(bytes32 id) public view returns (bool unspent) {
        return _unspent[id];
    }

    /**
     * @dev Query the lockId for a locked state.
     * @param id The state identifier.
     * @return lockId The lockId set when the lock was created.
     */
    function getLockId(bytes32 id) public view returns (bytes32 lockId) {
        return _locked[id];
    }

    /**
     * @dev Query the options for a lock.
     * @param lockId The lockId set when the lock was created.
     * @return options The options for the lock.
     */
    function getLockOptions(
        bytes32 lockId
    ) public view lockActive(lockId) returns (NotoLockOptions memory options) {
        return _locks[lockId].options;
    }

    /**
     * @dev Get current information about a lock.
     *      Reverts if the lock is not active.
     *
     * @param lockId The identifier of the lock.
     * @return info The information about the lock.
     */
    function getLock(
        bytes32 lockId
    ) public view override lockActive(lockId) returns (LockInfo memory info) {
        NotoLockInfo storage lock = _locks[lockId];
        return
            LockInfo({
                owner: lock.owner,
                spender: lock.spender,
                spendCommitment: lock.spendCommitment,
                cancelCommitment: lock.cancelCommitment
            });
    }

    /**
     * @dev Query whether the lock is currently active.
     *
     * @param lockId The unique identifier for the lock.
     * @return active Whether the lock is currently active.
     */
    function isLockActive(
        bytes32 lockId
    ) external view override returns (bool active) {
        return _locks[lockId].owner != address(0);
    }

    /**
     * @dev Get the content of a lock.
     *
     * @param lockId The unique identifier for the lock.
     * @return content The content of the lock.
     */
    function getLockContent(
        bytes32 lockId
    ) external view override lockActive(lockId) returns (bytes memory content) {
        return _locks[lockId].content;
    }

    /**
     * @dev Get current lockState ID for a lock
     *
     * @param lockId The identifier of the lock.
     * @return lockState The ID of the current lockState
     */
    function getLockState(
        bytes32 lockId
    ) external view returns (bytes32 lockState) {
        return _lockStates[lockId];
    }

    /**
     * @dev Spend states and create new states.
     *
     * @param txId A unique identifier for this transaction which must not have been used before.
     * @param inputs Array of zero or more states that the signer is authorized to spend.
     * @param outputs Array of zero or more new states to generate, for future transactions to spend.
     * @param signature A signature over the original request to the notary (opaque to the blockchain).
     * @param data Any additional transaction data (opaque to the blockchain).
     *
     * Emits a {Transfer} event.
     */
    function transfer(
        bytes32 txId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(txId) {
        _transfer(txId, inputs, outputs, signature, data);
    }

    /**
     * @dev Perform a transfer with no input states. Base implementation is identical
     *      to transfer(), but both methods can be overridden to provide different constraints.
     *
     * @param txId A unique identifier for this transaction which must not have been used before.
     * @param outputs Array of zero or more new states to generate.
     * @param signature A signature over the original request to the notary (opaque to the blockchain).
     * @param data Any additional transaction data (opaque to the blockchain).
     *
     * Emits a {Transfer} event.
     */
    function mint(
        bytes32 txId,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(txId) {
        _transfer(txId, new bytes32[](0), outputs, signature, data);
    }

    function _transfer(
        bytes32 txId,
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes calldata signature,
        bytes calldata data
    ) internal virtual {
        _processInputs(inputs);
        _processOutputs(outputs);
        emit Transfer(txId, msg.sender, inputs, outputs, signature, data);
    }

    /**
     * @dev Check that the inputs are all unspent, and remove them.
     */
    function _processInputs(bytes32[] memory inputs) internal {
        for (uint256 i = 0; i < inputs.length; ++i) {
            _processInput(inputs[i]);
        }
    }

    /**
     * @dev Check that the input is unspent, and remove it.
     */
    function _processInput(bytes32 input) internal {
        if (!isUnspent(input)) {
            revert NotoInvalidInput(input);
        }
        delete _unspent[input];
    }

    /**
     * @dev Check that the outputs are all new, and mark them as unspent.
     */
    function _processOutputs(bytes32[] memory outputs) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            _processOutput(outputs[i]);
        }
    }

    /**
     * @dev Check that an individual output is new, and mark it as unspent.
     */
    function _processOutput(bytes32 output) internal {
        if (isUnspent(output) || getLockId(output) != 0) {
            revert NotoInvalidOutput(output);
        }
        _unspent[output] = true;
    }

    function _unlockHash(
        bytes32 txId,
        bytes32[] memory lockedInputs,
        bytes32[] memory outputs,
        bytes memory data
    ) internal view returns (bytes32) {
        bytes32 structHash = keccak256(
            abi.encode(
                UNLOCK_TYPEHASH,
                txId,
                keccak256(abi.encodePacked(lockedInputs)),
                keccak256(abi.encodePacked(outputs)),
                keccak256(data)
            )
        );
        return _hashTypedDataV4(structHash);
    }

    /**
     * @dev Compute the lockId for given parameters (deterministic generation).
     *      This allows callers to predict the lockId before calling createLock().
     *
     * @param createInputs The inputs that will be passed to the createLock call
     * @return lockId The computed unique identifier for the lock.
     */
    function computeLockId(
        bytes calldata createInputs
    ) public view override returns (bytes32) {
        NotoCreateLockOperation memory lockOp = abi.decode(
            createInputs,
            (NotoCreateLockOperation)
        );
        return _computeLockId(lockOp.txId);
    }

    function _computeLockId(bytes32 txId) internal view returns (bytes32) {
        return keccak256(abi.encode(address(this), msg.sender, txId));
    }

    /**
     * @dev Create a new lock, by spending states and creating new locked states.
     *      Locks are identified by a unique lockId, which is generated deterministically.
     *      Locked states can be spent using spendLock(), or control of the lock can be
     *      delegated using delegateLock().
     *
     * @param createInputs The inputs must be a valid ABI encoded NotoCreateLockOperation structure
     * @param spendCommitment Commitment that constrains spendLock() to a specific spend operation (may be zero for unrestricted).
     * @param cancelCommitment Commitment that constrains cancelLock() to a specific cancel operation (may be zero for unrestricted).
     * @param data Any additional transaction data (opaque to the blockchain).
     * @return lockId The generated unique identifier for the lock.
     *
     * Emits a {LockCreated} event as defined in ILockableCapability.
     * Emits a {NotoLockCreated} event containing decoded Noto parameters.
     */
    function createLock(
        bytes calldata createInputs,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external virtual override onlyNotary returns (bytes32) {
        NotoCreateLockOperation memory lockOp = abi.decode(
            createInputs,
            (NotoCreateLockOperation)
        );
        bytes32 lockId = _computeLockId(lockOp.txId);
        NotoLockInfo storage lock = _locks[lockId];
        if (lock.owner != address(0)) {
            revert NotoDuplicateLock(lockId);
        }

        // Set the initial ownership, and immutable content of the lock
        lock.owner = msg.sender;
        lock.spender = msg.sender;
        lock.content = abi.encode(lockOp.content);

        _createLock(lockOp, spendCommitment, cancelCommitment, lockId, lock);

        emit LockCreated(
            lockId,
            msg.sender,
            msg.sender,
            spendCommitment,
            cancelCommitment,
            data
        );

        emit NotoLockCreated(
            lockOp.txId,
            lockId,
            msg.sender,
            lockOp.inputs,
            lockOp.outputs,
            lockOp.content,
            lockOp.newLockState,
            lockOp.proof,
            data
        );

        return lockId;
    }

    /**
     * @dev Update the current options for a lock.
     *
     * @param lockId Unique identifier for the lock.
     * @param updateInputs The inputs must be a valid ABI encoded NotoUpdateLockOperation structure
     * @param spendCommitment Replacement spend commitment (may be zero for unrestricted).
     * @param cancelCommitment Replacement cancel commitment (may be zero for unrestricted).
     * @param data Any additional transaction data (opaque to the blockchain).
     *
     * Emits a {LockUpdated} event as defined in ILockableCapability.
     * Emits a {NotoLockUpdated} event containing decoded Noto parameters.
     */
    function updateLock(
        bytes32 lockId,
        bytes calldata updateInputs,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external virtual override lockActive(lockId) onlyNotary {
        NotoUpdateLockOperation memory lockOp = abi.decode(
            updateInputs,
            (NotoUpdateLockOperation)
        );
        NotoLockInfo storage lock = _locks[lockId];
        if (lock.owner != lock.spender) {
            revert LockImmutable(lockId);
        }

        _updateLock(lockOp, spendCommitment, cancelCommitment, lockId, lock);

        emit LockUpdated(
            lockId,
            msg.sender,
            spendCommitment,
            cancelCommitment,
            data
        );

        bytes32[] memory lockContent = abi.decode(lock.content, (bytes32[]));

        emit NotoLockUpdated(
            lockOp.txId,
            lockId,
            msg.sender,
            lockContent,
            lockOp.oldLockState,
            lockOp.newLockState,
            lockOp.proof,
            data
        );
    }

    function _createLock(
        NotoCreateLockOperation memory lockOp,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes32 lockId,
        NotoLockInfo storage lock
    ) internal virtual {
        useTxId(lockOp.txId);

        _processInputs(lockOp.inputs);
        _processOutputs(lockOp.outputs);
        _processLockContent(lockId, lockOp.content);

        _processOutput(lockOp.newLockState);
        _lockStates[lockId] = lockOp.newLockState;

        lock.spendCommitment = spendCommitment;
        lock.cancelCommitment = cancelCommitment;

        if (lockOp.options.spendTxId != 0) {
            _setLockOptions(lockId, lock, lockOp.options);
        }
    }

    function _updateLock(
        NotoUpdateLockOperation memory lockOp,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes32 lockId,
        NotoLockInfo storage lock
    ) internal virtual {
        useTxId(lockOp.txId);

        _transitionLockState(lockId, lockOp.oldLockState, lockOp.newLockState);

        lock.spendCommitment = spendCommitment;
        lock.cancelCommitment = cancelCommitment;

        if (lockOp.options.spendTxId != 0) {
            _setLockOptions(lockId, lock, lockOp.options);
        }
    }

    function _setLockOptions(
        bytes32 lockId,
        NotoLockInfo storage lock,
        NotoLockOptions memory lockOptions
    ) internal virtual {
        NotoLockOptions memory oldOptions = lock.options;
        if (
            lockOptions.spendTxId != 0 &&
            oldOptions.spendTxId != lockOptions.spendTxId
        ) {
            if (_txIds[lockOptions.spendTxId]) {
                revert NotoDuplicateTransaction(lockOptions.spendTxId);
            }
            if (
                _lockTxIds[lockOptions.spendTxId] != 0 &&
                _lockTxIds[lockOptions.spendTxId] != lockId
            ) {
                revert NotoDuplicateSpendTransaction(lockOptions.spendTxId);
            }
            if (oldOptions.spendTxId != 0) {
                delete _lockTxIds[oldOptions.spendTxId];
            }
            _lockTxIds[lockOptions.spendTxId] = lockId;
        }
        lock.options = lockOptions;
    }

    /**
     * @dev Consume ("spend") the capability represented by this lock.
     *      The caller will either be the notary, or a delegated spender.
     *      If the caller is a delegated spender, the lock must have been prepared with a spendHash,
     *      and the provided state data must match the spendHash.
     *
     * @param lockId The identifier of the lock.
     * @param spendInputs Must be a valid ABI encoded NotoUnlockOperation struct
     * @param data Any additional transaction data (opaque to the blockchain).
     *
     * Emits a {LockSpent} event as defined in ILockableCapability.
     * Emits a {NotoLockSpent} event containing decoded Noto parameters.
     */
    function spendLock(
        bytes32 lockId,
        bytes calldata spendInputs,
        bytes calldata data
    ) external override lockActive(lockId) onlySpender(lockId) {
        NotoLockInfo storage lock = _locks[lockId];
        NotoUnlockOperation memory unlockOp = abi.decode(
            spendInputs,
            (NotoUnlockOperation)
        );
        bytes32 oldLockState = _lockStates[lockId];
        _spendLock(lockId, lock, lock.spendCommitment, unlockOp, oldLockState);
        emit LockSpent(lockId, msg.sender, data);
        emit NotoLockSpent(
            unlockOp.txId,
            lockId,
            msg.sender,
            unlockOp.inputs,
            unlockOp.outputs,
            unlockOp.data,
            oldLockState,
            unlockOp.proof,
            data
        );
    }

    /**
     * @dev Cancel this lock without performing its effect.
     *      The caller will either be the notary, or a delegated spender.
     *      If the caller is a delegated spender, the lock must have been prepared with a cancelHash,
     *      and the provided state data must match the cancelHash.
     *
     * @param lockId The identifier of the lock.
     * @param cancelInputs Must be a valid ABI encoded NotoUnlockOperation
     * @param data Any additional transaction data (opaque to the blockchain).
     *
     * Emits a {LockCancelled} event as defined in ILockableCapability.
     * Emits a {NotoLockCancelled} event containing decoded Noto parameters.
     */
    function cancelLock(
        bytes32 lockId,
        bytes calldata cancelInputs,
        bytes calldata data
    ) external override lockActive(lockId) onlySpender(lockId) {
        NotoLockInfo storage lock = _locks[lockId];
        NotoUnlockOperation memory unlockOp = abi.decode(
            cancelInputs,
            (NotoUnlockOperation)
        );
        bytes32 oldLockState = _lockStates[lockId];
        _spendLock(lockId, lock, lock.cancelCommitment, unlockOp, oldLockState);
        emit LockCancelled(lockId, msg.sender, data);
        emit NotoLockCancelled(
            unlockOp.txId,
            lockId,
            msg.sender,
            unlockOp.inputs,
            unlockOp.outputs,
            unlockOp.data,
            oldLockState,
            unlockOp.proof,
            data
        );
    }

    function _spendLock(
        bytes32 lockId,
        NotoLockInfo storage lock,
        bytes32 expectedHash,
        NotoUnlockOperation memory unlockOp,
        bytes32 oldLockState
    ) internal {
        NotoLockOptions storage options = lock.options;
        if (options.spendTxId != 0 && options.spendTxId != unlockOp.txId) {
            revert NotoInvalidTransaction(unlockOp.txId);
        }
        useTxId(unlockOp.txId);

        // If a specific unlock operation is expected, verify the hash matches
        if (expectedHash != 0) {
            bytes32 actualHash = _unlockHash(
                unlockOp.txId,
                unlockOp.inputs,
                unlockOp.outputs,
                unlockOp.data
            );
            if (actualHash != expectedHash) {
                revert NotoInvalidUnlockHash(expectedHash, actualHash);
            }
        }

        // The operation must unlock all the locked inputs.
        // Note only a length check here as _processLockedInputs checks the contents.
        bytes32[] memory lockContent = abi.decode(lock.content, (bytes32[]));
        if (lockContent.length != unlockOp.inputs.length) {
            revert NotoInvalidUnlockInputs(
                lockContent.length,
                unlockOp.inputs.length
            );
        }

        _processInput(oldLockState);
        _processLockedInputs(lockId, unlockOp.inputs);
        _processOutputs(unlockOp.outputs);

        delete _locks[lockId];
        delete _lockStates[lockId];

        return;
    }

    /**
     * @dev Delegate spending authority for a lock to a new address.
     *      The lock must have been prepared (both spendHash and cancelHash set).
     *
     * @param lockId The identifier of the lock.
     * @param newSpender The address of the new lock spender.
     * @param data ABI-encoded DelegateLockData struct.
     *
     * Emits a {LockDelegated} event.
     */
    function delegateLock(
        bytes32 lockId,
        bytes calldata delegateInputs,
        address newSpender,
        bytes calldata data
    ) external override lockActive(lockId) onlySpender(lockId) {
        NotoDelegateOperation memory delegateOp = abi.decode(
            delegateInputs,
            (NotoDelegateOperation)
        );
        NotoLockInfo storage lock = _locks[lockId];
        if (lock.spendCommitment == 0 || lock.cancelCommitment == 0) {
            revert NotoDelegationConditionsNotSet(lockId);
        }

        useTxId(delegateOp.txId);

        address previousSpender = lock.spender;
        lock.spender = newSpender;

        _transitionLockState(
            lockId,
            delegateOp.oldLockState,
            delegateOp.newLockState
        );

        emit LockDelegated(
            lockId,
            previousSpender,
            newSpender,
            msg.sender,
            data
        );

        emit NotoLockDelegated(
            delegateOp.txId,
            lockId,
            previousSpender,
            newSpender,
            delegateOp.oldLockState,
            delegateOp.newLockState,
            delegateOp.proof,
            data
        );
    }

    /**
     * @dev Validate and finalize the state transition for the lock state
     */
    function _transitionLockState(
        bytes32 lockId,
        bytes32 oldLockState,
        bytes32 newLockState
    ) internal {
        bytes32 currentLockState = _lockStates[lockId];
        if (currentLockState != oldLockState) {
            revert NotoInvalidLockState(lockId, oldLockState, currentLockState);
        }
        _processInput(oldLockState);
        _processOutput(newLockState);
        _lockStates[lockId] = newLockState;
    }

    /**
     * @dev Check the inputs are all locked.
     */
    function _checkLockedInputs(
        bytes32 lockId,
        bytes32[] memory inputs
    ) internal view {
        for (uint256 i = 0; i < inputs.length; ++i) {
            if (_locked[inputs[i]] != lockId) {
                revert NotoInvalidInput(inputs[i]);
            }
        }
    }

    /**
     * @dev Check the inputs are all locked, and remove them.
     */
    function _processLockedInputs(
        bytes32 lockId,
        bytes32[] memory inputs
    ) internal {
        for (uint256 i = 0; i < inputs.length; ++i) {
            _processLockedInput(lockId, inputs[i]);
        }
    }

    /**
     * @dev Check an individual inputs is locked, and remove it.
     */
    function _processLockedInput(bytes32 lockId, bytes32 input) internal {
        if (_locked[input] != lockId) {
            revert NotoInvalidInput(input);
        }
        delete _locked[input];
    }

    /**
     * @dev Check the outputs are all new, and mark them as locked.
     */
    function _processLockContent(
        bytes32 lockId,
        bytes32[] memory outputs
    ) internal {
        for (uint256 i = 0; i < outputs.length; ++i) {
            if (isUnspent(outputs[i]) || getLockId(outputs[i]) != 0) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _locked[outputs[i]] = lockId;
        }
    }
}
