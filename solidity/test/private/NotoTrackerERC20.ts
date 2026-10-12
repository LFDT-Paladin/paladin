import { expect } from "chai";
import { ZeroAddress } from "ethers";
import { ethers } from "hardhat";
import { NotoLocks, NotoTrackerERC20 } from "../../typechain-types";
import { randomBytes32 } from "../domains/noto/util";

describe("NotoTrackerERC20", function () {
  const prepared = { contractAddress: ZeroAddress, encodedCall: "0x" };

  async function deployTracker() {
    const [notary, alice, bob, delegate] = await ethers.getSigners();
    const tracker = (await (await ethers.getContractFactory("NotoTrackerERC20"))
      .connect(notary)
      .deploy("NOTO", "NOTO")) as NotoTrackerERC20;
    const trackerAddress = await tracker.getAddress();
    const locks = (await ethers.getContractFactory("NotoLocks")).attach(
      ethers.getCreateAddress({ from: trackerAddress, nonce: 1 }),
    ) as NotoLocks;
    return { tracker, locks, notary, alice, bob, delegate };
  }

  describe("pending balances", function () {
    it("releases the previous recipients when a lock is re-prepared", async function () {
      const { tracker, locks, notary, alice, bob, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateTransferLock(alice.address, lockId, alice.address, 60, [{ to: bob.address, amount: 60 }], "0x", prepared);
      await tracker.onPrepareUnlock(alice.address, lockId, [{ to: delegate.address, amount: 60 }], "0x", prepared);
      expect(await locks.pendingBalance(bob.address)).to.equal(0);
      expect(await locks.pendingBalance(delegate.address)).to.equal(60);
    });

    it("releases the previous recipients when a lock is re-prepared to burn", async function () {
      const { tracker, locks, notary, alice, bob } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateTransferLock(alice.address, lockId, alice.address, 60, [{ to: bob.address, amount: 60 }], "0x", prepared);
      await tracker.onPrepareBurnUnlock(alice.address, lockId, alice.address, 60, "0x", prepared);
      expect(await locks.pendingBalance(bob.address)).to.equal(0);
    });

    it("releases the previous recipients when a mint lock is re-prepared", async function () {
      const { tracker, locks, notary, alice, bob } = await deployTracker();
      const lockId = randomBytes32();
      await tracker.onCreateMintLock(notary.address, lockId, [{ to: alice.address, amount: 100 }], "0x", prepared);
      await tracker.onPrepareMintUnlock(notary.address, lockId, [{ to: bob.address, amount: 50 }], "0x", prepared);
      expect(await locks.pendingBalance(alice.address)).to.equal(0);
      expect(await locks.pendingBalance(bob.address)).to.equal(50);
    });

    it("releases the prepared recipients when the owner unlocks", async function () {
      const { tracker, locks, notary, alice, bob } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateTransferLock(alice.address, lockId, alice.address, 60, [{ to: bob.address, amount: 60 }], "0x", prepared);
      await tracker.onUnlock(alice.address, lockId, [{ to: alice.address, amount: 60 }], "0x", prepared);
      expect(await locks.pendingBalance(bob.address)).to.equal(0);
      expect(await locks.lockedBalance(alice.address)).to.equal(0);
    });
  });

  describe("delegated unlock", function () {
    it("mints to the recipients when a mint lock is spent", async function () {
      const { tracker, locks, notary, bob, delegate } = await deployTracker();
      const lockId = randomBytes32();
      const recipients = [{ to: bob.address, amount: 100 }];
      await tracker.onCreateMintLock(notary.address, lockId, recipients, "0x", prepared);
      expect(await locks.pendingBalance(bob.address)).to.equal(100);

      await tracker.handleDelegateUnlock(delegate.address, lockId, recipients, "0x");
      expect(await tracker.balanceOf(bob.address)).to.equal(100);
      expect(await tracker.totalSupply()).to.equal(100);
      expect(await locks.pendingBalance(bob.address)).to.equal(0);
    });

    it("mints nothing when a mint lock is cancelled", async function () {
      const { tracker, locks, notary, bob, delegate } = await deployTracker();
      const lockId = randomBytes32();
      await tracker.onCreateMintLock(notary.address, lockId, [{ to: bob.address, amount: 100 }], "0x", prepared);

      await tracker.handleDelegateUnlock(delegate.address, lockId, [], "0x");
      expect(await tracker.totalSupply()).to.equal(0);
      expect(await locks.pendingBalance(bob.address)).to.equal(0);
    });

    it("mints to the recipients of a re-prepared mint lock", async function () {
      const { tracker, notary, alice, bob, delegate } = await deployTracker();
      const lockId = randomBytes32();
      await tracker.onCreateMintLock(notary.address, lockId, [{ to: alice.address, amount: 100 }], "0x", prepared);
      const recipients = [{ to: bob.address, amount: 50 }];
      await tracker.onPrepareMintUnlock(notary.address, lockId, recipients, "0x", prepared);

      await tracker.handleDelegateUnlock(delegate.address, lockId, recipients, "0x");
      expect(await tracker.balanceOf(bob.address)).to.equal(50);
      expect(await tracker.totalSupply()).to.equal(50);
    });

    it("burns from the owner when a burn lock is spent", async function () {
      const { tracker, locks, notary, alice, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateBurnLock(alice.address, lockId, alice.address, 60, "0x", prepared);
      expect(await locks.lockedBalance(alice.address)).to.equal(60);

      await tracker.handleDelegateUnlock(delegate.address, lockId, [], "0x");
      expect(await tracker.balanceOf(alice.address)).to.equal(40);
      expect(await tracker.totalSupply()).to.equal(40);
      expect(await locks.lockedBalance(alice.address)).to.equal(0);
      expect(await locks.ownerOf(lockId)).to.equal(ZeroAddress);
    });

    it("returns the coins to the owner when a burn lock is cancelled", async function () {
      const { tracker, locks, notary, alice, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateBurnLock(alice.address, lockId, alice.address, 60, "0x", prepared);

      await tracker.handleDelegateUnlock(delegate.address, lockId, [{ to: alice.address, amount: 60 }], "0x");
      expect(await tracker.balanceOf(alice.address)).to.equal(100);
      expect(await tracker.totalSupply()).to.equal(100);
      expect(await locks.lockedBalance(alice.address)).to.equal(0);
      expect(await locks.ownerOf(lockId)).to.equal(ZeroAddress);
    });

    it("burns from the owner when a lock prepared to burn is spent", async function () {
      const { tracker, locks, notary, alice, bob, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateTransferLock(alice.address, lockId, alice.address, 60, [{ to: bob.address, amount: 60 }], "0x", prepared);
      await tracker.onPrepareBurnUnlock(alice.address, lockId, alice.address, 60, "0x", prepared);

      await tracker.handleDelegateUnlock(delegate.address, lockId, [], "0x");
      expect(await tracker.balanceOf(alice.address)).to.equal(40);
      expect(await tracker.balanceOf(bob.address)).to.equal(0);
      expect(await tracker.totalSupply()).to.equal(40);
      expect(await locks.lockedBalance(alice.address)).to.equal(0);
    });

    it("transfers to the recipients when a lock prepared to burn is re-prepared as a transfer and spent", async function () {
      const { tracker, locks, notary, alice, bob, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateBurnLock(alice.address, lockId, alice.address, 60, "0x", prepared);
      const recipients = [{ to: bob.address, amount: 60 }];
      await tracker.onPrepareUnlock(alice.address, lockId, recipients, "0x", prepared);

      await tracker.handleDelegateUnlock(delegate.address, lockId, recipients, "0x");
      expect(await tracker.balanceOf(alice.address)).to.equal(40);
      expect(await tracker.balanceOf(bob.address)).to.equal(60);
      expect(await tracker.totalSupply()).to.equal(100);
      expect(await locks.lockedBalance(alice.address)).to.equal(0);
    });
  });
});
