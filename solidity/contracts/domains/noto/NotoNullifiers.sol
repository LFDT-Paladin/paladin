// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {SmtLib} from "@iden3/contracts/contracts/lib/SmtLib.sol";
import {Noto} from "./Noto.sol";

uint256 constant MAX_SMT_DEPTH = 64;

contract NotoNullifiers is Noto {
    SmtLib.Data internal _commitmentsTree;
    using SmtLib for SmtLib.Data;

    uint64 public constant NotoVariantNullifiers = 0x0002;

    mapping(bytes32 => bool) private _nullifiers;

    function initialize(
        string memory name_,
        string memory symbol_,
        address notary_
    ) public virtual override initializer {
        super.initialize(name_, symbol_, notary_);
        _commitmentsTree.initialize(MAX_SMT_DEPTH);
    }

    function buildConfig(
        bytes calldata data
    ) external view override returns (bytes memory) {
        return
            _encodeConfig(
                NotoConfig_V1({
                    name: _name,
                    symbol: _symbol,
                    decimals: decimals(),
                    notary: notary,
                    variant: NotoVariantNullifiers,
                    data: data
                })
            );
    }

    function transfer(
        bytes32 txId,
        bytes32[] calldata inputs,
        bytes32[] calldata outputs,
        bytes calldata signature,
        bytes calldata data
    ) external virtual override onlyNotary txIdNotUsed(txId) {
        (uint256 root, bytes memory _signature) = abi.decode(
            signature,
            (uint256, bytes)
        );
        if (!_commitmentsTree.rootExists(root)) {
            revert NotoInvalidRoot(root);
        }
        _processNullifiers(inputs);
        _processOutputs(outputs);
        emit NotoTransfer(txId, inputs, outputs, _signature, data);
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
     * @param proof a proof over the original request to the notary (opaque to the blockchain)
     * @param data any additional transaction data (opaque to the blockchain)
     *
     * Emits a {NotoLock} event.
     */
    function lock(
        bytes32 txId,
        bytes32[] calldata nullifiers,
        bytes32[] calldata outputs,
        bytes32[] calldata lockedOutputs,
        bytes calldata proof,
        bytes calldata data
    ) public virtual override onlyNotary txIdNotUsed(txId) lockIdNotUsed(txId) {
        (uint256 root, bytes memory signature) = abi.decode(
            proof,
            (uint256, bytes)
        );
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
        bytes memory encoded = abi.encode(params);
        return uint256(keccak256(encoded));
    }
}
