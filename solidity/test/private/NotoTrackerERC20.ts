import { expect } from "chai";
import { ZeroAddress } from "ethers";
import { ethers } from "hardhat";
import { NotoTrackerERC20 } from "../../typechain-types";
import { deployUpgradeableContract } from "../helpers/deploy-upgradeable";
import { randomBytes32 } from "../domains/noto/util";

describe("NotoTrackerERC20", function () {
  const prepared = { contractAddress: ZeroAddress, encodedCall: "0x" };

  async function deployTracker() {
    const [notary, alice, bob, delegate] = await ethers.getSigners();
    const tracker = (
      (await deployUpgradeableContract(
        "NotoTrackerERC20",
        "initialize",
        ["NOTO", "NOTO", notary.address],
        alice,
      )) as NotoTrackerERC20
    ).connect(notary);
    return { tracker, notary, alice, bob, delegate };
  }

  describe("callers", function () {
    it("rejects every hook called by anyone but the notary", async function () {
      const { tracker, notary, alice, bob } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateTransferLock(alice.address, lockId, alice.address, 60, [{ to: bob.address, amount: 60 }], "0x", prepared);
      const recipients = [{ to: alice.address, amount: 60 }];

      const asAlice = tracker.connect(alice);
      const calls = [
        asAlice.onMint(notary.address, alice.address, 100, "0x", prepared),
        asAlice.onTransfer(alice.address, alice.address, bob.address, 10, "0x", prepared),
        asAlice.onBurn(alice.address, alice.address, 10, "0x", prepared),
        asAlice.onLock(alice.address, randomBytes32(), alice.address, 10, "0x", prepared),
        asAlice.onPrepareMintUnlock(notary.address, randomBytes32(), recipients, "0x", prepared),
        asAlice.onPrepareBurnUnlock(alice.address, lockId, alice.address, 60, "0x", prepared),
        asAlice.onUnlock(alice.address, lockId, recipients, "0x", prepared),
        asAlice.onPrepareUnlock(alice.address, lockId, recipients, "0x", prepared),
        asAlice.onCreateTransferLock(alice.address, randomBytes32(), alice.address, 10, recipients, "0x", prepared),
        asAlice.onCreateMintLock(notary.address, randomBytes32(), recipients, "0x", prepared),
        asAlice.onCreateBurnLock(alice.address, randomBytes32(), alice.address, 10, "0x", prepared),
        asAlice.onDelegateLock(alice.address, lockId, bob.address, prepared),
        asAlice.handleDelegateUnlock(alice.address, lockId, recipients, "0x"),
      ];
      for (const call of calls) {
        await expect(call).to.be.revertedWith("Sender is not the notary");
      }
    });
  });

  describe("ERC20", function () {
    it("rejects moving balances other than through the hooks", async function () {
      const { tracker, notary, alice, bob } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);

      const asAlice = tracker.connect(alice);
      await expect(asAlice.transfer(bob.address, 10)).to.be.revertedWith("Direct transfers are disabled: balances only change through Noto hooks");
      await expect(asAlice.approve(bob.address, 10)).to.be.revertedWith("Direct transfers are disabled: balances only change through Noto hooks");
      await expect(tracker.connect(bob).transferFrom(alice.address, bob.address, 10)).to.be.revertedWith(
        "Direct transfers are disabled: balances only change through Noto hooks",
      );
      expect(await tracker.balanceOf(alice.address)).to.equal(100);
    });
  });

  describe("pending balances", function () {
    it("releases the previous recipients when a lock is re-prepared", async function () {
      const { tracker, notary, alice, bob, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateTransferLock(alice.address, lockId, alice.address, 60, [{ to: bob.address, amount: 60 }], "0x", prepared);
      await tracker.onPrepareUnlock(alice.address, lockId, [{ to: delegate.address, amount: 60 }], "0x", prepared);
      expect(await tracker.pendingBalance(bob.address)).to.equal(0);
      expect(await tracker.pendingBalance(delegate.address)).to.equal(60);
    });

    it("releases the previous recipients when a lock is re-prepared to burn", async function () {
      const { tracker, notary, alice, bob } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateTransferLock(alice.address, lockId, alice.address, 60, [{ to: bob.address, amount: 60 }], "0x", prepared);
      await tracker.onPrepareBurnUnlock(alice.address, lockId, alice.address, 60, "0x", prepared);
      expect(await tracker.pendingBalance(bob.address)).to.equal(0);
    });

    it("releases the previous recipients when a mint lock is re-prepared", async function () {
      const { tracker, notary, alice, bob } = await deployTracker();
      const lockId = randomBytes32();
      await tracker.onCreateMintLock(notary.address, lockId, [{ to: alice.address, amount: 100 }], "0x", prepared);
      await tracker.onPrepareMintUnlock(notary.address, lockId, [{ to: bob.address, amount: 50 }], "0x", prepared);
      expect(await tracker.pendingBalance(alice.address)).to.equal(0);
      expect(await tracker.pendingBalance(bob.address)).to.equal(50);
    });

    it("releases the prepared recipients when the owner unlocks", async function () {
      const { tracker, notary, alice, bob } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateTransferLock(alice.address, lockId, alice.address, 60, [{ to: bob.address, amount: 60 }], "0x", prepared);
      await tracker.onUnlock(alice.address, lockId, [{ to: alice.address, amount: 60 }], "0x", prepared);
      expect(await tracker.pendingBalance(bob.address)).to.equal(0);
      expect(await tracker.lockedBalance(alice.address)).to.equal(0);
    });
  });

  describe("delegated unlock", function () {
    it("mints to the recipients when a mint lock is spent", async function () {
      const { tracker, notary, bob, delegate } = await deployTracker();
      const lockId = randomBytes32();
      const recipients = [{ to: bob.address, amount: 100 }];
      await tracker.onCreateMintLock(notary.address, lockId, recipients, "0x", prepared);
      expect(await tracker.pendingBalance(bob.address)).to.equal(100);

      await tracker.handleDelegateUnlock(delegate.address, lockId, recipients, "0x");
      expect(await tracker.balanceOf(bob.address)).to.equal(100);
      expect(await tracker.totalSupply()).to.equal(100);
      expect(await tracker.pendingBalance(bob.address)).to.equal(0);
    });

    it("mints nothing when a mint lock is cancelled", async function () {
      const { tracker, notary, bob, delegate } = await deployTracker();
      const lockId = randomBytes32();
      await tracker.onCreateMintLock(notary.address, lockId, [{ to: bob.address, amount: 100 }], "0x", prepared);

      await tracker.handleDelegateUnlock(delegate.address, lockId, [], "0x");
      expect(await tracker.totalSupply()).to.equal(0);
      expect(await tracker.pendingBalance(bob.address)).to.equal(0);
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
      const { tracker, notary, alice, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateBurnLock(alice.address, lockId, alice.address, 60, "0x", prepared);
      expect(await tracker.lockedBalance(alice.address)).to.equal(60);

      await tracker.handleDelegateUnlock(delegate.address, lockId, [], "0x");
      expect(await tracker.balanceOf(alice.address)).to.equal(40);
      expect(await tracker.totalSupply()).to.equal(40);
      expect(await tracker.lockedBalance(alice.address)).to.equal(0);
      expect(await tracker.lockOwner(lockId)).to.equal(ZeroAddress);
    });

    it("returns the coins to the owner when a burn lock is cancelled", async function () {
      const { tracker, notary, alice, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateBurnLock(alice.address, lockId, alice.address, 60, "0x", prepared);

      await tracker.handleDelegateUnlock(delegate.address, lockId, [{ to: alice.address, amount: 60 }], "0x");
      expect(await tracker.balanceOf(alice.address)).to.equal(100);
      expect(await tracker.totalSupply()).to.equal(100);
      expect(await tracker.lockedBalance(alice.address)).to.equal(0);
      expect(await tracker.lockOwner(lockId)).to.equal(ZeroAddress);
    });

    it("burns from the owner when a lock prepared to burn is spent", async function () {
      const { tracker, notary, alice, bob, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateTransferLock(alice.address, lockId, alice.address, 60, [{ to: bob.address, amount: 60 }], "0x", prepared);
      await tracker.onPrepareBurnUnlock(alice.address, lockId, alice.address, 60, "0x", prepared);

      await tracker.handleDelegateUnlock(delegate.address, lockId, [], "0x");
      expect(await tracker.balanceOf(alice.address)).to.equal(40);
      expect(await tracker.balanceOf(bob.address)).to.equal(0);
      expect(await tracker.totalSupply()).to.equal(40);
      expect(await tracker.lockedBalance(alice.address)).to.equal(0);
    });

    it("transfers to the recipients when a lock prepared to burn is re-prepared as a transfer and spent", async function () {
      const { tracker, notary, alice, bob, delegate } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const lockId = randomBytes32();
      await tracker.onCreateBurnLock(alice.address, lockId, alice.address, 60, "0x", prepared);
      const recipients = [{ to: bob.address, amount: 60 }];
      await tracker.onPrepareUnlock(alice.address, lockId, recipients, "0x", prepared);

      await tracker.handleDelegateUnlock(delegate.address, lockId, recipients, "0x");
      expect(await tracker.balanceOf(alice.address)).to.equal(40);
      expect(await tracker.balanceOf(bob.address)).to.equal(60);
      expect(await tracker.totalSupply()).to.equal(100);
      expect(await tracker.lockedBalance(alice.address)).to.equal(0);
    });
  });

  describe("upgrades", function () {
    it("lets the deployer upgrade and keeps the tracked balances", async function () {
      const { tracker, notary, alice } = await deployTracker();
      await tracker.onMint(notary.address, alice.address, 100, "0x", prepared);
      const newImplementation = await (await ethers.getContractFactory("NotoTrackerERC20")).deploy();

      await tracker.connect(alice).upgradeToAndCall(newImplementation, "0x");
      expect(await tracker.balanceOf(alice.address)).to.equal(100);
    });

    it("rejects upgrades by anyone but the deployer", async function () {
      const { tracker, notary } = await deployTracker();
      const newImplementation = await (await ethers.getContractFactory("NotoTrackerERC20")).deploy();

      await expect(tracker.upgradeToAndCall(newImplementation, "0x"))
        .to.be.revertedWithCustomError(tracker, "OwnableUnauthorizedAccount")
        .withArgs(notary.address);
    });

    it("rejects initializing the implementation directly", async function () {
      const { notary } = await deployTracker();
      const implementation = await (await ethers.getContractFactory("NotoTrackerERC20")).deploy();

      await expect(implementation.initialize("NOTO", "NOTO", notary.address))
        .to.be.revertedWithCustomError(implementation, "InvalidInitialization");
    });
  });
});
