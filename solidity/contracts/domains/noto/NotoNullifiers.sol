// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {SmtLib} from "@iden3/contracts/contracts/lib/SmtLib.sol";
import {Noto} from "./Noto.sol";

uint256 constant MAX_SMT_DEPTH = 64;

contract NotoNullifiers is Noto {
    using SmtLib for SmtLib.Data;

    SmtLib.Data private _commitmentsTree;

    // see domains/noto/pkg/types/config.go for the convention
    // on the variant field
    uint64 public constant NotoVariantV2Nullifiers = 0x0102;

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
                    name: name(),
                    symbol: symbol(),
                    decimals: decimals(),
                    notary: notary,
                    variant: NotoVariantV2Nullifiers,
                    data: data
                })
            );
    }

    function _decodeProof(
        bytes memory proof
    ) internal pure returns (uint256 root, bytes memory signature) {
        return abi.decode(proof, (uint256, bytes));
    }

    function _transfer(
        bytes32 txId,
        bytes32[] memory inputs,
        bytes32[] memory outputs,
        bytes calldata proof,
        bytes calldata data
    ) internal virtual override {
        (uint256 root, bytes memory _signature) = _decodeProof(proof);
        _requireValidRoot(root);
        _processNullifiers(inputs);
        _processOutputs(outputs);
        emit Transfer(txId, msg.sender, inputs, outputs, _signature, data);
    }

    function _createLock(
        NotoCreateLockArgs memory args,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes32 lockId,
        NotoLockInfo storage lock
    ) internal virtual override {
        useTxId(args.txId);

        (uint256 root, ) = _decodeProof(args.proof);
        _requireValidRoot(root);

        _processNullifiers(args.inputs);
        _processOutputs(args.outputs);
        _processLockContents(lockId, args.contents);

        super._processOutput(args.newLockState);
        _setLockState(lockId, args.newLockState);

        lock.spendCommitment = spendCommitment;
        lock.cancelCommitment = cancelCommitment;

        if (args.options.spendTxId != 0) {
            _setLockOptions(lockId, lock, args.options);
        }
    }

    function _updateLock(
        NotoUpdateLockArgs memory args,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes32 lockId,
        NotoLockInfo storage lock
    ) internal virtual override {
        if (args.proof.length == 0) {
            revert NotoInvalidProof(args.proof);
        }
        (uint256 root, ) = _decodeProof(args.proof);
        _requireValidRoot(root);
        super._updateLock(
            args,
            spendCommitment,
            cancelCommitment,
            lockId,
            lock
        );
    }

    /**
     * @dev Lock states use base Noto _unspent tracking so they can be consumed on
     *      updateLock, delegateLock, and spendLock. Only regular transaction outputs
     *      are stored in the append-only commitment tree.
     */
    function _transitionLockState(
        bytes32 lockId,
        bytes32 oldLockState,
        bytes32 newLockState
    ) internal virtual override {
        bytes32 currentLockState = getLockState(lockId);
        if (currentLockState != oldLockState) {
            revert NotoInvalidLockState(lockId, oldLockState, currentLockState);
        }
        super._processInput(oldLockState);
        super._processOutput(newLockState);
        _setLockState(lockId, newLockState);
    }

    /**
     * @dev Append-only commitment tree for regular (unlocked) outputs. Lock states
     *      and locked contents are tracked via base Noto _unspent / _locked instead.
     */
    function _processOutput(bytes32 output) internal virtual override {
        uint256 outputUint = uint256(output);
        if (_existsAsUnlocked(outputUint) || getLockId(output) != bytes32(0)) {
            revert NotoInvalidOutput(output);
        }
        _commitmentsTree.addLeaf(outputUint, outputUint);
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
            _nullifiers[inputNullifiers[i]] = true;
        }
    }

    function _requireValidRoot(uint256 root) internal view {
        if (!_commitmentsTree.rootExists(root)) {
            revert NotoInvalidRoot(root);
        }
    }

    /**
     * @dev Check the existence of a UTXO in the commitments tree. we take a shortcut
     *      by checking the list of nodes by their node hash, because the commitments
     *      tree is append-only, no updates or deletions are allowed. As a result, all
     *      nodes in the list are valid leaf nodes, aka there are no orphaned nodes.
     */
    function _existsAsUnlocked(uint256 utxo) internal view returns (bool) {
        uint256 nodeHash = _getLeafNodeHash(utxo, utxo);
        SmtLib.Node memory node = _commitmentsTree.getNode(nodeHash);
        return node.nodeType != SmtLib.NodeType.EMPTY;
    }

    function _getLeafNodeHash(
        uint256 index,
        uint256 value
    ) internal pure returns (uint256) {
        uint256[3] memory params = [index, value, uint256(1)];
        bytes memory encoded = abi.encode(params);
        return uint256(keccak256(encoded));
    }
}
