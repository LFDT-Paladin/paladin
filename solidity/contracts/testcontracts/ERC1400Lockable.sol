// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "../domains/interfaces/ILockableCapability.sol";

/**
 * @title ERC1400Lockable
 * @dev Partitioned security token (ERC-1400 / ERC-1410 style) that implements
 *      ILockableCapability for per-partition balance locking.
 *
 *      Each holder's balance is split across one or more partitions
 *      (a.k.a. tranches). Locks reserve a token amount within a *specific*
 *      partition and can be spent or cancelled by the current spender.
 *
 *      Total balance (ERC20 `balanceOf`) is the sum of partition balances.
 *      Locked partition balances are tracked as a subset of the holder's
 *      partition balance and block transfers/redemptions that would dip
 *      into them.
 *
 *      This contract intentionally mirrors the style of ERC20Lockable - it
 *      is a minimal, self-contained example. It implements the partition
 *      semantics of ERC-1410 and a small slice of ERC-1594 (issuance to a
 *      partition) so the lock lifecycle can be exercised end-to-end.
 */
contract ERC1400Lockable is ERC20, Ownable, ILockableCapability {
    // ---------------------------------------------------------------------
    // Lock content / storage
    // ---------------------------------------------------------------------

    struct CreateLockInputs {
        bytes32 partition;
        uint256 amount;
        address recipient;
    }

    struct TokenLock {
        address owner;
        address spender;
        CreateLockInputs content;
        bytes32 spendCommitment;
        bytes32 cancelCommitment;
        bool active;
    }

    // ERC-8074 type selector for the bytes returned by getLockContent.
    // bytes4(keccak256("ERC1400LockContent(bytes32 partition,uint256 amount,address recipient)"))
    string public constant ERC1400LockContentType =
        "ERC1400LockContent(bytes32 partition,uint256 amount,address recipient)";
    bytes4 public constant ERC1400LockContentSelector =
        bytes4(keccak256(bytes(ERC1400LockContentType)));

    mapping(bytes32 => TokenLock) private _locks;
    uint256 private _lockCounter;

    // ---------------------------------------------------------------------
    // Partition (ERC-1410) storage
    // ---------------------------------------------------------------------

    // Per-holder, per-partition balance.
    mapping(address => mapping(bytes32 => uint256)) private _balancesByPartition;

    // Per-holder, per-partition locked amount (always <= balance in partition).
    mapping(address => mapping(bytes32 => uint256))
        private _lockedBalancesByPartition;

    // List of partitions a holder currently has a balance in.
    mapping(address => bytes32[]) private _partitionsOf;
    mapping(address => mapping(bytes32 => uint256)) private _partitionIndex;
    mapping(address => mapping(bytes32 => bool)) private _hasPartition;

    // ---------------------------------------------------------------------
    // Events (ERC-1410 / ERC-1594 subset)
    // ---------------------------------------------------------------------

    event IssuedByPartition(
        bytes32 indexed partition,
        address indexed to,
        uint256 value,
        bytes data
    );

    event TransferByPartition(
        bytes32 indexed fromPartition,
        address operator,
        address indexed from,
        address indexed to,
        uint256 value,
        bytes data,
        bytes operatorData
    );

    event RedeemedByPartition(
        bytes32 indexed partition,
        address indexed operator,
        address indexed from,
        uint256 value,
        bytes data
    );

    // ---------------------------------------------------------------------
    // Errors
    // ---------------------------------------------------------------------

    error InvalidAmount(uint256 amount);
    error InvalidPartition(bytes32 partition);
    error InsufficientPartitionBalance(
        address from,
        bytes32 partition,
        uint256 available,
        uint256 requested
    );
    /// @dev Plain ERC20 transfer paths are disabled; use transferByPartition.
    error UsePartitionedTransfer();

    // ---------------------------------------------------------------------
    // ERC-1066-style transfer status codes
    //
    // We use a small subset relevant to the locking sample. Implementations
    // are free to extend this with eligibility / KYC / lockup-period codes.
    // ---------------------------------------------------------------------

    /// @dev Transfer would succeed under current state.
    bytes1 public constant TRANSFER_SUCCESS = 0xA0;
    /// @dev Transfer blocked because the value exceeds the unlocked partition balance.
    bytes1 public constant TRANSFER_INSUFFICIENT_LOCKED = 0xA3;
    /// @dev Transfer blocked because the partition balance (ignoring locks) is too small.
    bytes1 public constant TRANSFER_INSUFFICIENT_BALANCE = 0xA4;
    /// @dev Transfer blocked because the partition is the zero partition.
    bytes1 public constant TRANSFER_INVALID_PARTITION = 0xA8;

    // ---------------------------------------------------------------------
    // Construction / issuance
    // ---------------------------------------------------------------------

    constructor(
        string memory name,
        string memory symbol
    ) ERC20(name, symbol) Ownable(_msgSender()) {}

    /**
     * @dev Mint `amount` tokens into `partition` for `to`.
     *      ERC-1594-style partitioned issuance.
     */
    function issueByPartition(
        bytes32 partition,
        address to,
        uint256 amount,
        bytes calldata data
    ) external onlyOwner {
        require(partition != bytes32(0), InvalidPartition(partition));
        _mint(to, amount);
        _addToPartition(to, partition, amount);
        emit IssuedByPartition(partition, to, amount, data);
    }

    // ---------------------------------------------------------------------
    // ERC-1410 partition views
    // ---------------------------------------------------------------------

    function balanceOfByPartition(
        bytes32 partition,
        address holder
    ) external view returns (uint256) {
        return _balancesByPartition[holder][partition];
    }

    function lockedBalanceOfByPartition(
        bytes32 partition,
        address holder
    ) external view returns (uint256) {
        return _lockedBalancesByPartition[holder][partition];
    }

    function unlockedBalanceOfByPartition(
        bytes32 partition,
        address holder
    ) external view returns (uint256) {
        return
            _balancesByPartition[holder][partition] -
            _lockedBalancesByPartition[holder][partition];
    }

    function partitionsOf(
        address holder
    ) external view returns (bytes32[] memory) {
        return _partitionsOf[holder];
    }

    // ---------------------------------------------------------------------
    // ERC-1410-style transferability check
    // ---------------------------------------------------------------------

    /**
     * @dev Pre-flight check: would a transferByPartition succeed right now?
     *      Returns an ERC-1066-style status code, an extra reason byte/bytes32,
     *      and the destination partition (here, always the same as the source).
     *
     *      This is the single source of truth for transfer rules - the actual
     *      transfer path goes through the same `_canTransferByPartition` hook
     *      and reverts with a matching error if the check fails.
     */
    function canTransferByPartition(
        address from,
        address /* to */,
        bytes32 partition,
        uint256 value,
        bytes calldata /* data */
    )
        external
        view
        returns (bytes1 code, bytes32 reason, bytes32 destinationPartition)
    {
        (, code, reason) = _canTransferByPartition(from, partition, value);
        destinationPartition = partition;
    }

    // ---------------------------------------------------------------------
    // ERC-1410 partition transfer
    // ---------------------------------------------------------------------

    /**
     * @dev Transfer `amount` from msg.sender's `partition` to `to`'s same partition.
     *      Reverts if the unlocked partition balance is insufficient.
     */
    function transferByPartition(
        bytes32 partition,
        address to,
        uint256 amount,
        bytes calldata data
    ) external returns (bytes32) {
        _transferByPartition(msg.sender, to, partition, amount);
        emit TransferByPartition(
            partition,
            msg.sender,
            msg.sender,
            to,
            amount,
            data,
            ""
        );
        // ERC-1410 returns the destination partition. We keep them aligned.
        return partition;
    }

    /**
     * @dev Redeem (burn) `amount` from msg.sender's `partition`.
     *      ERC-1594-style partitioned redemption. Reverts if the unlocked
     *      partition balance is insufficient, so locked tokens can never be
     *      burned out from under a lock.
     */
    function redeemByPartition(
        bytes32 partition,
        uint256 amount,
        bytes calldata data
    ) external {
        _redeemByPartition(msg.sender, partition, amount);
        emit RedeemedByPartition(partition, msg.sender, msg.sender, amount, data);
    }

    // ---------------------------------------------------------------------
    // ILockableCapability
    // ---------------------------------------------------------------------

    /**
     * @dev Create a lock reserving an amount of tokens within a partition.
     *      The lock's owner and initial spender are both msg.sender.
     */
    function createLock(
        bytes calldata createArgs,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external override returns (bytes32 lockId) {
        CreateLockInputs memory inputs = abi.decode(
            createArgs,
            (CreateLockInputs)
        );
        require(inputs.amount > 0, InvalidAmount(inputs.amount));

        // Reuse the single transferability check so lock creation
        // can't reserve more than the unlocked partition balance.
        (bool ok, bytes1 code, ) = _canTransferByPartition(
            msg.sender,
            inputs.partition,
            inputs.amount
        );
        if (!ok) {
            if (code == TRANSFER_INVALID_PARTITION) {
                revert InvalidPartition(inputs.partition);
            }
            uint256 unlocked = _balancesByPartition[msg.sender][
                inputs.partition
            ] - _lockedBalancesByPartition[msg.sender][inputs.partition];
            revert InsufficientPartitionBalance(
                msg.sender,
                inputs.partition,
                unlocked,
                inputs.amount
            );
        }

        lockId = keccak256(
            abi.encode(address(this), msg.sender, ++_lockCounter)
        );

        _locks[lockId] = TokenLock({
            owner: msg.sender,
            spender: msg.sender,
            content: inputs,
            spendCommitment: spendCommitment,
            cancelCommitment: cancelCommitment,
            active: true
        });

        _lockedBalancesByPartition[msg.sender][inputs.partition] += inputs.amount;

        emit LockCreated(
            lockId,
            msg.sender,
            msg.sender,
            spendCommitment,
            cancelCommitment,
            data
        );
    }

    /**
     * @dev Update an active, owner-controlled lock. Replaces both commitments.
     *      The implementation-specific `updateArgs` are unused here - amount,
     *      partition, and recipient are fixed at creation.
     */
    function updateLock(
        bytes32 lockId,
        bytes calldata /* updateArgs */,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external override {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );
        require(lock.spender == lock.owner, LockImmutable(lockId));

        lock.spendCommitment = spendCommitment;
        lock.cancelCommitment = cancelCommitment;

        emit LockUpdated(
            lockId,
            lock.owner,
            spendCommitment,
            cancelCommitment,
            data
        );
    }

    /**
     * @dev Delegate spending authority for the lock.
     */
    function delegateLock(
        bytes32 lockId,
        bytes calldata /* delegateArgs */,
        address newSpender,
        bytes calldata data
    ) external override {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );

        address oldSpender = lock.spender;
        lock.spender = newSpender;

        emit LockDelegated(lockId, oldSpender, newSpender, data);
    }

    /**
     * @dev Spend the lock: transfer the locked amount from owner to recipient,
     *      preserving the partition.
     */
    function spendLock(
        bytes32 lockId,
        bytes calldata /* spendArgs */,
        bytes calldata data
    ) external override {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );

        bytes32 partition = lock.content.partition;
        uint256 amount = lock.content.amount;
        address owner = lock.owner;
        address recipient = lock.content.recipient;

        // Release the lock reservation before transferring so the partition
        // transfer can validate against the unlocked balance.
        _lockedBalancesByPartition[owner][partition] -= amount;
        delete _locks[lockId];

        _transferByPartition(owner, recipient, partition, amount);

        emit TransferByPartition(
            partition,
            msg.sender,
            owner,
            recipient,
            amount,
            data,
            ""
        );
        emit LockSpent(lockId, msg.sender, data);
    }

    /**
     * @dev Cancel the lock: release the reserved amount back to unlocked balance.
     */
    function cancelLock(
        bytes32 lockId,
        bytes calldata /* cancelArgs */,
        bytes calldata data
    ) external override {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );

        _lockedBalancesByPartition[lock.owner][lock.content.partition] -= lock
            .content
            .amount;
        delete _locks[lockId];

        emit LockCancelled(lockId, msg.sender, data);
    }

    function getLock(
        bytes32 lockId
    ) external view override returns (LockInfo memory) {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        return
            LockInfo({
                owner: lock.owner,
                spender: lock.spender,
                spendCommitment: lock.spendCommitment,
                cancelCommitment: lock.cancelCommitment
            });
    }

    /**
     * @dev Return the ERC-8074 encoded lock content
     *      (partition + amount + recipient) for an active lock.
     */
    function getLockContent(
        bytes32 lockId
    ) external view returns (bytes memory) {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        return
            bytes.concat(ERC1400LockContentSelector, abi.encode(lock.content));
    }

    function isLockActive(
        bytes32 lockId
    ) external view override returns (bool) {
        return _locks[lockId].active;
    }

    // ---------------------------------------------------------------------
    // ERC20 overrides
    //
    // Plain transfer/transferFrom would bypass partition accounting and let
    // partition balances drift relative to the aggregate ERC20 balance.
    // Disable them and require callers to use transferByPartition.
    // ---------------------------------------------------------------------

    function transfer(
        address,
        uint256
    ) public pure override returns (bool) {
        revert UsePartitionedTransfer();
    }

    function transferFrom(
        address,
        address,
        uint256
    ) public pure override returns (bool) {
        revert UsePartitionedTransfer();
    }

    // ---------------------------------------------------------------------
    // Internal: partition bookkeeping
    //
    // Locks are scoped to a single partition, so all lock enforcement is
    // per-partition (in _canTransferByPartition). There is no meaningful
    // "locked across all partitions" quantity: an unlocked balance in
    // partition A can never satisfy a debit from partition B. The base ERC20
    // movers are only ever reached through the partition-scoped paths below
    // (plain transfer/transferFrom are disabled and there is no burn other
    // than redeemByPartition), so no _update override is needed.
    // ---------------------------------------------------------------------

    function _transferByPartition(
        address from,
        address to,
        bytes32 partition,
        uint256 amount
    ) internal {
        (bool ok, bytes1 code, ) = _canTransferByPartition(
            from,
            partition,
            amount
        );
        if (!ok) {
            if (code == TRANSFER_INVALID_PARTITION) {
                revert InvalidPartition(partition);
            }
            // Both TRANSFER_INSUFFICIENT_LOCKED and TRANSFER_INSUFFICIENT_BALANCE
            // map to the same on-chain revert: the unlocked partition balance
            // is too small.
            uint256 unlocked = _balancesByPartition[from][partition] -
                _lockedBalancesByPartition[from][partition];
            revert InsufficientPartitionBalance(
                from,
                partition,
                unlocked,
                amount
            );
        }

        // Move partition balance first, then defer to ERC20 _transfer for
        // total-balance accounting. The two stay in lockstep.
        _removeFromPartition(from, partition, amount);
        _addToPartition(to, partition, amount);
        _transfer(from, to, amount);
    }

    function _redeemByPartition(
        address from,
        bytes32 partition,
        uint256 amount
    ) internal {
        (bool ok, bytes1 code, ) = _canTransferByPartition(
            from,
            partition,
            amount
        );
        if (!ok) {
            if (code == TRANSFER_INVALID_PARTITION) {
                revert InvalidPartition(partition);
            }
            uint256 unlocked = _balancesByPartition[from][partition] -
                _lockedBalancesByPartition[from][partition];
            revert InsufficientPartitionBalance(
                from,
                partition,
                unlocked,
                amount
            );
        }

        // Burn from the partition first, then defer to ERC20 _burn for
        // total-balance accounting. The two stay in lockstep.
        _removeFromPartition(from, partition, amount);
        _burn(from, amount);
    }

    /**
     * @dev Single source of truth for partition-level transferability.
     *      Returns `(ok, statusCode, reason)`:
     *        - ok               = (statusCode == TRANSFER_SUCCESS)
     *        - statusCode       = ERC-1066-style byte (see constants above)
     *        - reason           = supplementary info; here, the unlocked
     *                             partition balance when blocked, or 0 on success.
     *
     *      Overriders (e.g. for KYC, lockup periods, controller pauses) should
     *      compose with super by short-circuiting on their own block reasons
     *      before delegating to this base check.
     */
    function _canTransferByPartition(
        address from,
        bytes32 partition,
        uint256 amount
    ) internal view virtual returns (bool ok, bytes1 code, bytes32 reason) {
        if (partition == bytes32(0)) {
            return (false, TRANSFER_INVALID_PARTITION, bytes32(0));
        }
        uint256 balance = _balancesByPartition[from][partition];
        uint256 locked = _lockedBalancesByPartition[from][partition];
        uint256 unlocked = balance - locked;
        if (amount > balance) {
            return (
                false,
                TRANSFER_INSUFFICIENT_BALANCE,
                bytes32(unlocked)
            );
        }
        if (amount > unlocked) {
            return (
                false,
                TRANSFER_INSUFFICIENT_LOCKED,
                bytes32(unlocked)
            );
        }
        return (true, TRANSFER_SUCCESS, bytes32(0));
    }

    function _addToPartition(
        address holder,
        bytes32 partition,
        uint256 amount
    ) internal {
        if (!_hasPartition[holder][partition]) {
            _partitionIndex[holder][partition] = _partitionsOf[holder].length;
            _partitionsOf[holder].push(partition);
            _hasPartition[holder][partition] = true;
        }
        _balancesByPartition[holder][partition] += amount;
    }

    function _removeFromPartition(
        address holder,
        bytes32 partition,
        uint256 amount
    ) internal {
        _balancesByPartition[holder][partition] -= amount;
        if (
            _balancesByPartition[holder][partition] == 0 &&
            _lockedBalancesByPartition[holder][partition] == 0 &&
            _hasPartition[holder][partition]
        ) {
            uint256 idx = _partitionIndex[holder][partition];
            uint256 last = _partitionsOf[holder].length - 1;
            if (idx != last) {
                bytes32 moved = _partitionsOf[holder][last];
                _partitionsOf[holder][idx] = moved;
                _partitionIndex[holder][moved] = idx;
            }
            _partitionsOf[holder].pop();
            delete _partitionIndex[holder][partition];
            _hasPartition[holder][partition] = false;
        }
    }
}
