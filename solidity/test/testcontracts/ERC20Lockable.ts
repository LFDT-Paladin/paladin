import { expect } from "chai";
import type { ContractTransactionResponse, Signer } from "ethers";
import { ethers } from "hardhat";
import type { ERC20Lockable } from "../../typechain-types";
import {
  createTokenLock,
  decodeLockContent,
  encodeCreateLockInputs,
  ERC20_LOCK_CONTENT_TYPE,
  lockIdOf,
  LockKind,
} from "../helpers/erc20-lockable";

describe("ERC20Lockable (ERC-8316 example)", function () {
  let token: ERC20Lockable;
  let owner: Signer;
  let alice: Signer;
  let bob: Signer;
  let charlie: Signer;

  let aliceAddress: string;
  let bobAddress: string;
  let charlieAddress: string;

  beforeEach(async function () {
    [owner, alice, bob, charlie] = await ethers.getSigners();

    aliceAddress = await alice.getAddress();
    bobAddress = await bob.getAddress();
    charlieAddress = await charlie.getAddress();

    const ERC20LockableFactory =
      await ethers.getContractFactory("ERC20Lockable");
    token = (await ERC20LockableFactory.connect(owner).deploy(
      "LockableToken",
      "LOCK",
    )) as ERC20Lockable;

    await token.connect(owner).mint(aliceAddress, ethers.parseEther("1000"));
  });

  describe("Basic ERC20 functionality", function () {
    it("should have correct name and symbol", async function () {
      expect(await token.name()).to.equal("LockableToken");
      expect(await token.symbol()).to.equal("LOCK");
    });

    it("should allow normal transfers when no locks exist", async function () {
      await token.connect(alice).transfer(bobAddress, ethers.parseEther("100"));

      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("900"),
      );
      expect(await token.balanceOf(bobAddress)).to.equal(
        ethers.parseEther("100"),
      );
    });

    it("should show correct locked and unlocked balances", async function () {
      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
      expect(await token.unlockedBalanceOf(aliceAddress)).to.equal(
        ethers.parseEther("1000"),
      );
    });
  });

  describe("Lock creation and management", function () {
    it("should create a lock successfully", async function () {
      const amount = ethers.parseEther("200");

      const lockId = await createTokenLock(token, alice, amount, bobAddress);

      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(amount);
      expect(await token.unlockedBalanceOf(aliceAddress)).to.equal(
        ethers.parseEther("800"),
      );
      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("1000"),
      );

      expect(lockId).to.be.a("string");
      expect(lockId).to.have.length.greaterThan(0);
      expect(await token.isLockActive(lockId)).to.equal(true);

      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.owner).to.equal(aliceAddress);
      expect(lockInfo.spender).to.equal(aliceAddress);

      const content = decodeLockContent(await token.getLockContent(lockId));
      expect(content).to.deep.equal({
        selector: ethers.id(ERC20_LOCK_CONTENT_TYPE).slice(0, 10),
        kind: LockKind.Transfer,
        amount,
        recipient: bobAddress,
      });
    });

    it("should state the lock's content in ERC20LockCreated", async function () {
      const amount = ethers.parseEther("200");
      const tx = await token
        .connect(alice)
        .createLock(
          encodeCreateLockInputs(amount, bobAddress),
          ethers.ZeroHash,
          ethers.ZeroHash,
          "0x",
        );
      const lockId = await lockIdOf(token, tx);

      await expect(tx)
        .to.emit(token, "ERC20LockCreated")
        .withArgs(lockId, aliceAddress, LockKind.Transfer, amount, bobAddress, "0x");
    });

    it("should revert when locking for transfer to the zero address", async function () {
      await expect(
        token
          .connect(alice)
          .createLock(
            encodeCreateLockInputs(ethers.parseEther("1"), ethers.ZeroAddress),
            ethers.ZeroHash,
            ethers.ZeroHash,
            "0x",
          ),
      ).to.be.revertedWithCustomError(token, "InvalidRecipient");
    });

    it("should revert when locking a zero amount", async function () {
      const createInputs = encodeCreateLockInputs(0n, bobAddress);

      await expect(
        token
          .connect(alice)
          .createLock(createInputs, ethers.ZeroHash, ethers.ZeroHash, "0x"),
      ).to.be.revertedWithCustomError(token, "InvalidAmount");
    });

    it("should revert when trying to lock more than unlocked balance", async function () {
      const amount = ethers.parseEther("1500"); // more than alice's balance
      const createInputs = encodeCreateLockInputs(amount, bobAddress);

      await expect(
        token
          .connect(alice)
          .createLock(createInputs, ethers.ZeroHash, ethers.ZeroHash, "0x"),
      ).to.be.revertedWithCustomError(token, "ERC20InsufficientBalance");
    });

    it("should prevent transfers when insufficient unlocked balance", async function () {
      const lockAmount = ethers.parseEther("800");

      await createTokenLock(token, alice, lockAmount, bobAddress);

      await expect(
        token.connect(alice).transfer(bobAddress, ethers.parseEther("300")),
      ).to.be.revertedWithCustomError(token, "ERC20InsufficientBalance");

      await token.connect(alice).transfer(bobAddress, ethers.parseEther("100"));
      expect(await token.unlockedBalanceOf(aliceAddress)).to.equal(
        ethers.parseEther("100"),
      );
    });
  });

  describe("Lock update and mutability", function () {
    let lockId: string;
    const newSpendCommitment = ethers.keccak256(
      ethers.toUtf8Bytes("new spend commitment"),
    );

    beforeEach(async function () {
      lockId = await createTokenLock(
        token,
        alice,
        ethers.parseEther("300"),
        bobAddress,
      );
    });

    it("should allow the owner to update commitments while owner-controlled", async function () {
      await token
        .connect(alice)
        .updateLock(lockId, "0x", newSpendCommitment, ethers.ZeroHash, "0x");

      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.spendCommitment).to.equal(newSpendCommitment);
    });

    it("should revert update from a non-spender", async function () {
      await expect(
        token
          .connect(bob)
          .updateLock(lockId, "0x", newSpendCommitment, ethers.ZeroHash, "0x"),
      ).to.be.revertedWithCustomError(token, "LockUnauthorized");
    });
  });

  describe("Lock spending", function () {
    let lockId: string;

    beforeEach(async function () {
      lockId = await createTokenLock(
        token,
        alice,
        ethers.parseEther("300"),
        bobAddress,
      );
    });

    it("should spend lock successfully and delete it", async function () {
      const initialAliceBalance = await token.balanceOf(aliceAddress);
      const initialBobBalance = await token.balanceOf(bobAddress);

      await expect(token.connect(alice).spendLock(lockId, "0x", "0x"))
        .to.emit(token, "LockSpent")
        .withArgs(lockId, aliceAddress, "0x");

      expect(await token.balanceOf(aliceAddress)).to.equal(
        initialAliceBalance - ethers.parseEther("300"),
      );
      expect(await token.balanceOf(bobAddress)).to.equal(
        initialBobBalance + ethers.parseEther("300"),
      );

      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
      expect(await token.isLockActive(lockId)).to.equal(false);
      await expect(token.getLock(lockId)).to.be.revertedWithCustomError(
        token,
        "LockNotActive",
      );
    });

    it("should revert if non-spender tries to spend", async function () {
      await expect(
        token.connect(bob).spendLock(lockId, "0x", "0x"),
      ).to.be.revertedWithCustomError(token, "LockUnauthorized");
    });

    it("should revert when spending an already-spent lock", async function () {
      await token.connect(alice).spendLock(lockId, "0x", "0x");
      await expect(
        token.connect(alice).spendLock(lockId, "0x", "0x"),
      ).to.be.revertedWithCustomError(token, "LockNotActive");
    });
  });

  describe("Lock cancellation", function () {
    let lockId: string;

    beforeEach(async function () {
      lockId = await createTokenLock(
        token,
        alice,
        ethers.parseEther("300"),
        bobAddress,
      );
    });

    it("should cancel lock successfully and delete it", async function () {
      const initialAliceBalance = await token.balanceOf(aliceAddress);

      await expect(token.connect(alice).cancelLock(lockId, "0x", "0x"))
        .to.emit(token, "LockCancelled")
        .withArgs(lockId, aliceAddress, "0x");

      // No token transfer occurred; the reservation is released.
      expect(await token.balanceOf(aliceAddress)).to.equal(initialAliceBalance);
      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
      expect(await token.unlockedBalanceOf(aliceAddress)).to.equal(
        initialAliceBalance,
      );

      await expect(token.getLock(lockId)).to.be.revertedWithCustomError(
        token,
        "LockNotActive",
      );
    });

    it("should revert if non-spender tries to cancel", async function () {
      await expect(
        token.connect(bob).cancelLock(lockId, "0x", "0x"),
      ).to.be.revertedWithCustomError(token, "LockUnauthorized");
    });
  });

  describe("Lock delegation", function () {
    let lockId: string;

    beforeEach(async function () {
      lockId = await createTokenLock(
        token,
        alice,
        ethers.parseEther("300"),
        bobAddress,
      );
    });

    it("should delegate authority and let the new spender finalize", async function () {
      await expect(
        token.connect(alice).delegateLock(lockId, "0x", charlieAddress, "0x"),
      )
        .to.emit(token, "LockDelegated")
        .withArgs(lockId, aliceAddress, charlieAddress, "0x");

      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.spender).to.equal(charlieAddress);
      expect(lockInfo.owner).to.equal(aliceAddress); // owner unchanged

      await token.connect(charlie).spendLock(lockId, "0x", "0x");
      expect(await token.balanceOf(bobAddress)).to.equal(
        ethers.parseEther("300"),
      );
    });

    it("should revert when delegating to the zero address", async function () {
      await expect(
        token
          .connect(alice)
          .delegateLock(lockId, "0x", ethers.ZeroAddress, "0x"),
      ).to.be.revertedWithCustomError(token, "LockInvalidSpender");
    });

    it("should strip authority from the previous spender after delegation", async function () {
      await token
        .connect(alice)
        .delegateLock(lockId, "0x", charlieAddress, "0x");

      await expect(
        token.connect(alice).spendLock(lockId, "0x", "0x"),
      ).to.be.revertedWithCustomError(token, "LockUnauthorized");
    });

    it("should make commitments immutable once delegated to a non-owner", async function () {
      await token
        .connect(alice)
        .delegateLock(lockId, "0x", charlieAddress, "0x");

      await expect(
        token
          .connect(charlie)
          .updateLock(
            lockId,
            "0x",
            ethers.keccak256(ethers.toUtf8Bytes("new spend commitment")),
            ethers.ZeroHash,
            "0x",
          ),
      ).to.be.revertedWithCustomError(token, "LockImmutable");
    });

    it("should allow updates again once authority returns to the owner", async function () {
      await token
        .connect(alice)
        .delegateLock(lockId, "0x", charlieAddress, "0x");
      await token
        .connect(charlie)
        .delegateLock(lockId, "0x", aliceAddress, "0x");

      const newSpendCommitment = ethers.keccak256(
        ethers.toUtf8Bytes("restored owner control"),
      );
      await token
        .connect(alice)
        .updateLock(lockId, "0x", newSpendCommitment, ethers.ZeroHash, "0x");

      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.spendCommitment).to.equal(newSpendCommitment);
    });
  });

  describe("Burn locks", function () {
    const amount = ethers.parseEther("300");
    let lockId: string;
    let tx: ContractTransactionResponse;

    beforeEach(async function () {
      tx = await token
        .connect(alice)
        .createBurnLock(amount, ethers.ZeroHash, ethers.ZeroHash, "0x");
      lockId = await lockIdOf(token, tx);
    });

    it("should reserve the caller's balance, the caller owning the lock", async function () {
      await expect(tx)
        .to.emit(token, "ERC20LockCreated")
        .withArgs(lockId, aliceAddress, LockKind.Burn, amount, ethers.ZeroAddress, "0x");
      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(amount);

      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.owner).to.equal(aliceAddress);
      expect(lockInfo.spender).to.equal(aliceAddress);

      const content = decodeLockContent(await token.getLockContent(lockId));
      expect([content.kind, content.amount, content.recipient]).to.deep.equal([
        LockKind.Burn,
        amount,
        ethers.ZeroAddress,
      ]);
    });

    it("should burn the reserved amount when spent by a delegated spender", async function () {
      const supply = await token.totalSupply();

      await token.connect(alice).delegateLock(lockId, "0x", charlieAddress, "0x");
      await token.connect(charlie).spendLock(lockId, "0x", "0x");

      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("700"),
      );
      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
      expect(await token.totalSupply()).to.equal(supply - amount);
    });

    it("should release the reservation when cancelled", async function () {
      await token.connect(alice).cancelLock(lockId, "0x", "0x");

      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("1000"),
      );
      expect(await token.lockedBalanceOf(aliceAddress)).to.equal(0);
    });

    it("should reserve only the caller's unlocked balance", async function () {
      await expect(
        token
          .connect(alice)
          .createBurnLock(ethers.parseEther("701"), ethers.ZeroHash, ethers.ZeroHash, "0x"),
      ).to.be.revertedWithCustomError(token, "ERC20InsufficientBalance");
    });
  });

  describe("Mint locks", function () {
    const amount = ethers.parseEther("50");
    let ownerAddress: string;
    let lockId: string;
    let tx: ContractTransactionResponse;

    beforeEach(async function () {
      ownerAddress = await owner.getAddress();
      tx = await token
        .connect(owner)
        .createMintLock(bobAddress, amount, ethers.ZeroHash, ethers.ZeroHash, "0x");
      lockId = await lockIdOf(token, tx);
    });

    it("should fix the mint without reserving a balance", async function () {
      await expect(tx)
        .to.emit(token, "ERC20LockCreated")
        .withArgs(lockId, ownerAddress, LockKind.Mint, amount, bobAddress, "0x");
      expect(await token.lockedBalanceOf(ownerAddress)).to.equal(0);

      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.owner).to.equal(ownerAddress);
      expect(lockInfo.spender).to.equal(ownerAddress);
    });

    it("should mint to the recipient when spent", async function () {
      const supply = await token.totalSupply();

      await token.connect(owner).spendLock(lockId, "0x", "0x");

      expect(await token.balanceOf(bobAddress)).to.equal(amount);
      expect(await token.totalSupply()).to.equal(supply + amount);
    });

    it("should mint nothing when cancelled", async function () {
      const supply = await token.totalSupply();

      await token.connect(owner).cancelLock(lockId, "0x", "0x");

      expect(await token.totalSupply()).to.equal(supply);
      expect(await token.isLockActive(lockId)).to.equal(false);
    });

    it("should be created only by the token owner", async function () {
      await expect(
        token
          .connect(bob)
          .createMintLock(bobAddress, amount, ethers.ZeroHash, ethers.ZeroHash, "0x"),
      ).to.be.revertedWithCustomError(token, "OwnableUnauthorizedAccount");
    });
  });
});
