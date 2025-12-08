// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {SmtLib} from "@iden3/contracts/contracts/lib/SmtLib.sol";
import {Keccak256Hasher} from "@iden3/contracts/contracts/lib/hash/KeccakHasher.sol";
import {IHasher} from "@iden3/contracts/contracts/interfaces/IHasher.sol";
import {Noto} from "./Noto.sol";
import {console} from "hardhat/console.sol";

uint256 constant MAX_SMT_DEPTH = 64;

contract NotoNullifiers is Noto {
    SmtLib.Data internal _commitmentsTree;
    using SmtLib for SmtLib.Data;

    mapping(bytes32 => bool) private _nullifiers;
    IHasher private _hasher;

    function initialize(
        string memory name_,
        string memory symbol_,
        address notary_
    ) public virtual override initializer {
        super.initialize(name_, symbol_, notary_);
        _commitmentsTree.initialize(MAX_SMT_DEPTH);
        _hasher = new Keccak256Hasher();
        _commitmentsTree.setHasher(_hasher);
        NotoVariantDefault = 0x0002;
    }

    function transfer(
        bytes32 txId,
        bytes32[] calldata nullifiers,
        bytes32[] calldata outputs,
        uint256 root,
        bytes calldata signature,
        bytes calldata data
    ) external virtual onlyNotary {
        if (!_commitmentsTree.rootExists(root)) {
            revert NotoInvalidRoot(root);
        }
        _processNullifiers(nullifiers);
        _processOutputs(outputs);
        emit NotoTransfer(txId, nullifiers, outputs, signature, data);
    }

    /**
     * @dev Lock some value so it cannot be spent until it is unlocked.
     *      The lockId is computed deterministically from the txId.
     *
     * @param txId a unique identifier for this transaction which must not have been used before
     * @param nullifiers array of zero or more outputs of a previous function call against this
     *      contract that have not yet been spent, and the signer is authorized to spend
     * @param outputs array of zero or more new outputs to generate, for future transactions to spend
     * @param lockedOutputs array of zero or more locked outputs to generate, which will be tied to the lock ID
     * @param root a historical or current root of the commitments tree containing the inputs represented by the nullifiers
     * @param signature a signature over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoLock} event.
     */
    function lock(
        bytes32 txId,
        bytes32[] calldata nullifiers,
        bytes32[] calldata outputs,
        bytes32[] calldata lockedOutputs,
        uint256 root,
        bytes calldata signature,
        bytes calldata data
    ) public virtual onlyNotary txIdNotUsed(txId) lockIdNotUsed(txId) {
        if (!_commitmentsTree.rootExists(root)) {
            revert NotoInvalidRoot(root);
        }

        bytes32 lockId = computeLockId(txId);

        _processNullifiers(nullifiers);
        _processOutputs(outputs);
        _processLockedOutputs(lockId, lockedOutputs);

        LockInfo storage lockInfo = _locks[lockId];
        lockInfo.owner = msg.sender;

        emit NotoLock(
            txId,
            lockId,
            nullifiers,
            outputs,
            lockedOutputs,
            signature,
            data
        );
    }

    /**
     * @dev Check the inputs are nullifiers that have not been used, and mark them as used
     */
    function _processNullifiers(
        bytes32[] memory inputNullifiers
    ) internal virtual {
        for (uint256 i = 0; i < inputNullifiers.length; ++i) {
            if (_nullifiers[inputNullifiers[i]]) {
                revert NotoInvalidInput(inputNullifiers[i]);
            }
            // record the nullifier as used
            _nullifiers[inputNullifiers[i]] = true;
        }
    }

    /**
     * @dev Check the outputs are all new UTXOs, and add them to the commitments tree
     */
    function _processOutputs(bytes32[] memory outputs) internal override {
        for (uint256 i = 0; i < outputs.length; ++i) {
            uint256 output = uint256(outputs[i]);
            if (
                existsAsUnlocked(output) || getLockId(outputs[i]) != bytes32(0)
            ) {
                revert NotoInvalidOutput(outputs[i]);
            }
            _commitmentsTree.addLeaf(output, output);
        }
    }

    // check the existence of a UTXO in the commitments tree. we take a shortcut
    // by checking the list of nodes by their node hash, because the commitments
    // tree is append-only, no updates or deletions are allowed. As a result, all
    // nodes in the list are valid leaf nodes, aka there are no orphaned nodes.
    function existsAsUnlocked(uint256 utxo) internal view returns (bool) {
        uint256 nodeHash = getLeafNodeHash(utxo, utxo);
        SmtLib.Node memory node = _commitmentsTree.getNode(nodeHash);
        return node.nodeType != SmtLib.NodeType.EMPTY;
    }

    function getLeafNodeHash(
        uint256 index,
        uint256 value
    ) internal view returns (uint256) {
        uint256[3] memory params = [index, value, uint256(1)];
        return _hasher.hash3(params);
    }
}
