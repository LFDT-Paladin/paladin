// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/ERC20Burnable.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "../domains/interfaces/IERC8316.sol";

/**
 * @title ERC20Lockable
 * @dev A transparent ERC-20 token that implements ERC-8316 (Programmable
 *      Settlement Locks) for balance locking.
 *
 *      A lock is one of three kinds:
 *      - Transfer: reserves an amount of the owner's balance for transfer to
 *        a fixed recipient.
 *      - Burn: reserves an amount of a holder's balance for burning.
 *      - Mint: fixes a mint of an amount to a recipient.
 *      The current spender may spend the lock (performing its operation) or
 *      cancel it (releasing any reservation back to the owner). Locked
 *      balances are tracked as a subset of the owner's total balance and
 *      cannot be moved by ordinary transfers.
 *
 *      Because the underlying token is fully transparent, the operation is
 *      completely determined by the lock content (kind, amount and recipient)
 *      captured at creation, which ERC20LockCreated states alongside the
 *      standard LockCreated. There is no hidden state to bind, so this example
 *      leaves `spendCommitment` and `cancelCommitment` unrestricted (zero) and
 *      performs no commitment validation in spendLock/cancelLock. A
 *      privacy-preserving implementation would instead bind proofs, encrypted
 *      values, or nullifiers into the commitments and validate the supplied
 *      args against them.
 */
contract ERC20Lockable is ERC20, ERC20Burnable, Ownable, IERC8316 {
    struct CreateLockInputs {
        uint256 amount;
        address recipient;
    }

    enum LockKind {
        Transfer,
        Burn,
        Mint
    }

    struct LockContent {
        LockKind kind;
        uint256 amount;
        address recipient; // zero for a burn
    }

    struct TokenLock {
        address owner;
        address spender;
        LockContent content;
        bytes32 spendCommitment;
        bytes32 cancelCommitment;
        bool active;
    }

    // Type selector for the bytes returned by getLockContent (an
    // implementation-specific extension, ERC-8074 encoded).
    // bytes4(keccak256("ERC20LockContent(uint8 kind,uint256 amount,address recipient)"))
    string public constant ERC20LockContentType =
        "ERC20LockContent(uint8 kind,uint256 amount,address recipient)";
    bytes4 public constant ERC20LockContentSelector =
        bytes4(keccak256(bytes(ERC20LockContentType)));

    mapping(bytes32 => TokenLock) private _locks;
    mapping(address => uint256) private _lockedBalances;
    uint256 private _lockCounter;

    /**
     * @dev Emitted after LockCreated, with the lock's content.
     *      An implementation-specific extension.
     */
    event ERC20LockCreated(
        bytes32 indexed lockId,
        address indexed owner,
        LockKind indexed kind,
        uint256 amount,
        address recipient,
        bytes data
    );

    error InvalidAmount(uint256 amount);
    error InvalidRecipient(address recipient);

    constructor(
        string memory name,
        string memory symbol
    ) ERC20(name, symbol) Ownable(_msgSender()) {}

    function mint(address to, uint256 amount) public onlyOwner {
        _mint(to, amount);
    }

    /**
     * @dev Override the balance update hook to ensure locked balances cannot be
     *      moved by ordinary transfers.
     */
    function _update(
        address from,
        address to,
        uint256 value
    ) internal override {
        if (from != address(0)) {
            uint256 unlockedBalance = balanceOf(from) - _lockedBalances[from];
            if (value > unlockedBalance) {
                revert ERC20InsufficientBalance(from, unlockedBalance, value);
            }
        }
        super._update(from, to, value);
    }

    /**
     * @dev Get the locked balance for an account.
     */
    function lockedBalanceOf(address account) external view returns (uint256) {
        return _lockedBalances[account];
    }

    /**
     * @dev Get the unlocked (available) balance for an account.
     */
    function unlockedBalanceOf(
        address account
    ) external view returns (uint256) {
        return balanceOf(account) - _lockedBalances[account];
    }

    /**
     * @dev Create a lock reserving `amount` of the caller's balance for
     *      transfer to `recipient`.
     */
    function createLock(
        bytes calldata createArgs,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external returns (bytes32 lockId) {
        CreateLockInputs memory inputs = abi.decode(createArgs, (CreateLockInputs));
        require(inputs.recipient != address(0), InvalidRecipient(inputs.recipient));
        _reserve(msg.sender, inputs.amount);
        lockId = _newLock(
            msg.sender,
            msg.sender,
            LockContent(LockKind.Transfer, inputs.amount, inputs.recipient),
            spendCommitment,
            cancelCommitment,
            data
        );
    }

    /**
     * @dev Create a lock reserving `amount` of the caller's balance for burning.
     */
    function createBurnLock(
        uint256 amount,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external returns (bytes32 lockId) {
        _reserve(msg.sender, amount);
        lockId = _newLock(
            msg.sender,
            msg.sender,
            LockContent(LockKind.Burn, amount, address(0)),
            spendCommitment,
            cancelCommitment,
            data
        );
    }

    /**
     * @dev Create a lock fixing a mint of `amount` to `to`, made when the lock
     *      is spent. The caller, the token owner, owns the lock.
     */
    function createMintLock(
        address to,
        uint256 amount,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external onlyOwner returns (bytes32 lockId) {
        require(amount > 0, InvalidAmount(amount));
        require(to != address(0), InvalidRecipient(to));
        lockId = _newLock(
            msg.sender,
            msg.sender,
            LockContent(LockKind.Mint, amount, to),
            spendCommitment,
            cancelCommitment,
            data
        );
    }

    /**
     * @dev Reserve `amount` of `holder`'s unlocked balance.
     */
    function _reserve(address holder, uint256 amount) private {
        require(amount > 0, InvalidAmount(amount));
        uint256 unlockedBalance = balanceOf(holder) - _lockedBalances[holder];
        require(
            amount <= unlockedBalance,
            ERC20InsufficientBalance(holder, unlockedBalance, amount)
        );
        _lockedBalances[holder] += amount;
    }

    function _newLock(
        address owner,
        address spender,
        LockContent memory content,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) private returns (bytes32 lockId) {
        lockId = keccak256(abi.encode(address(this), owner, ++_lockCounter));

        _locks[lockId] = TokenLock({
            owner: owner,
            spender: spender,
            content: content,
            spendCommitment: spendCommitment,
            cancelCommitment: cancelCommitment,
            active: true
        });

        emit LockCreated(
            lockId,
            owner,
            spender,
            spendCommitment,
            cancelCommitment,
            data
        );
        emit ERC20LockCreated(
            lockId,
            owner,
            content.kind,
            content.amount,
            content.recipient,
            data
        );
    }

    /**
     * @dev Update an existing lock's commitments (only while owner-controlled).
     */
    function updateLock(
        bytes32 lockId,
        bytes calldata /* updateArgs */,
        bytes32 spendCommitment,
        bytes32 cancelCommitment,
        bytes calldata data
    ) external {
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
     * @dev Delegate spending authority to another address.
     */
    function delegateLock(
        bytes32 lockId,
        bytes calldata /* delegateArgs */,
        address newSpender,
        bytes calldata data
    ) external {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );
        require(
            newSpender != address(0),
            LockInvalidSpender(lockId, newSpender)
        );

        address oldSpender = lock.spender;
        lock.spender = newSpender;

        emit LockDelegated(lockId, oldSpender, newSpender, data);
    }

    /**
     * @dev Spend the lock: transfer to the recipient, burn, or mint.
     */
    function spendLock(
        bytes32 lockId,
        bytes calldata /* spendArgs */,
        bytes calldata data
    ) external {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );

        LockContent memory content = lock.content;
        address owner = lock.owner;

        // Update state before the transfer (which may reenter) per ERC-8316
        // implementation-safety guidance.
        if (content.kind != LockKind.Mint) {
            _lockedBalances[owner] -= content.amount;
        }
        delete _locks[lockId];

        if (content.kind == LockKind.Transfer) {
            _transfer(owner, content.recipient, content.amount);
        } else if (content.kind == LockKind.Burn) {
            _burn(owner, content.amount);
        } else {
            _mint(content.recipient, content.amount);
        }

        emit LockSpent(lockId, msg.sender, data);
    }

    /**
     * @dev Cancel the lock, releasing any reserved tokens back to the owner.
     */
    function cancelLock(
        bytes32 lockId,
        bytes calldata /* cancelArgs */,
        bytes calldata data
    ) external {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        require(
            msg.sender == lock.spender,
            LockUnauthorized(lockId, lock.spender, msg.sender)
        );

        if (lock.content.kind != LockKind.Mint) {
            _lockedBalances[lock.owner] -= lock.content.amount;
        }
        delete _locks[lockId];

        emit LockCancelled(lockId, msg.sender, data);
    }

    /**
     * @dev Get ERC-8316 lock information for an active lock.
     */
    function getLock(bytes32 lockId) external view returns (LockInfo memory) {
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
     * @dev Get the implementation-specific lock content (kind, amount and recipient),
     *      prefixed with a type selector. This is an extension (not included in ERC-8316).
     */
    function getLockContent(
        bytes32 lockId
    ) external view returns (bytes memory) {
        TokenLock storage lock = _locks[lockId];
        require(lock.active, LockNotActive(lockId));
        return bytes.concat(ERC20LockContentSelector, abi.encode(lock.content));
    }

    /**
     * @dev Query whether a lock is currently active.
     */
    function isLockActive(bytes32 lockId) external view returns (bool) {
        return _locks[lockId].active;
    }
}
