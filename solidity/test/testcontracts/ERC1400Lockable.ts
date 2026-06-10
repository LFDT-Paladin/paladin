import { expect } from "chai";
import type { Signer } from "ethers";
import { ethers } from "hardhat";
import type { ERC1400Lockable } from "../../typechain-types";
import {
  createTokenLock,
  encodeCreateLockInputs,
  ERC1400_CREATE_LOCK_INPUTS_TYPE,
  ERC1400_LOCK_CONTENT_TYPE,
  PARTITION_A,
  PARTITION_B,
} from "../helpers/erc1400-lockable";

describe("ERC1400Lockable", function () {
  let token: ERC1400Lockable;
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

    // Deploy ERC1400Lockable token
    const ERC1400LockableFactory = await ethers.getContractFactory(
      "ERC1400Lockable",
    );
    token = (await ERC1400LockableFactory.connect(owner).deploy(
      "LockableSecurity",
      "SEC",
    )) as ERC1400Lockable;

    // Seed alice's partition A balance
    await token
      .connect(owner)
      .issueByPartition(
        PARTITION_A,
        aliceAddress,
        ethers.parseEther("1000"),
        "0x",
      );
  });

  describe("Basic partition functionality", function () {
    it("should have correct name and symbol", async function () {
      expect(await token.name()).to.equal("LockableSecurity");
      expect(await token.symbol()).to.equal("SEC");
    });

    it("should reflect issuance in partition and aggregate balances", async function () {
      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("1000"),
      );
      expect(
        await token.balanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(ethers.parseEther("1000"));
      expect(await token.partitionsOf(aliceAddress)).to.deep.equal([
        PARTITION_A,
      ]);
    });

    it("should disable plain ERC20 transfers", async function () {
      await expect(
        token.connect(alice).transfer(bobAddress, ethers.parseEther("100")),
      ).to.be.revertedWithCustomError(token, "UsePartitionedTransfer");
    });

    it("should allow transferByPartition when no locks exist", async function () {
      await token
        .connect(alice)
        .transferByPartition(
          PARTITION_A,
          bobAddress,
          ethers.parseEther("100"),
          "0x",
        );

      expect(
        await token.balanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(ethers.parseEther("900"));
      expect(
        await token.balanceOfByPartition(PARTITION_A, bobAddress),
      ).to.equal(ethers.parseEther("100"));
      expect(await token.balanceOf(bobAddress)).to.equal(
        ethers.parseEther("100"),
      );
    });

    it("should show correct locked/unlocked balances per partition", async function () {
      expect(
        await token.lockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(0);
      expect(
        await token.unlockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(ethers.parseEther("1000"));
    });

    it("should track multiple partitions independently", async function () {
      await token
        .connect(owner)
        .issueByPartition(
          PARTITION_B,
          aliceAddress,
          ethers.parseEther("500"),
          "0x",
        );

      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("1500"),
      );
      expect(
        await token.balanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(ethers.parseEther("1000"));
      expect(
        await token.balanceOfByPartition(PARTITION_B, aliceAddress),
      ).to.equal(ethers.parseEther("500"));

      const partitions = await token.partitionsOf(aliceAddress);
      expect(partitions).to.have.length(2);
      expect(partitions).to.include(PARTITION_A);
      expect(partitions).to.include(PARTITION_B);
    });
  });

  describe("Lock creation and management", function () {
    it("should create a lock successfully in a partition", async function () {
      const amount = ethers.parseEther("200");

      const lockId = await createTokenLock(
        token,
        alice,
        PARTITION_A,
        amount,
        bobAddress,
      );

      // Verify per-partition locked accounting
      expect(
        await token.lockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(amount);
      expect(
        await token.unlockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(ethers.parseEther("800"));
      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("1000"),
      );

      // Verify we got a valid lock ID
      expect(lockId).to.be.a("string");
      expect(lockId).to.have.length.greaterThan(0);

      // Verify lock info
      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.owner).to.equal(aliceAddress);
      expect(lockInfo.spender).to.equal(aliceAddress);

      // Verify ERC-8074-encoded lock content
      const content = await token.getLockContent(lockId);
      const expectedSelector = ethers
        .id(ERC1400_LOCK_CONTENT_TYPE)
        .slice(0, 10);
      expect(content.slice(0, 10)).to.equal(expectedSelector);
      const [inputs] = ethers.AbiCoder.defaultAbiCoder().decode(
        [ERC1400_CREATE_LOCK_INPUTS_TYPE],
        "0x" + content.slice(10),
      );
      expect(inputs.partition).to.equal(PARTITION_A);
      expect(inputs.amount).to.equal(amount);
      expect(inputs.recipient).to.equal(bobAddress);
    });

    it("should revert when trying to lock more than unlocked partition balance", async function () {
      const amount = ethers.parseEther("1500"); // More than alice's partition A balance
      const createInputs = encodeCreateLockInputs(
        PARTITION_A,
        amount,
        bobAddress,
      );

      await expect(
        token
          .connect(alice)
          .createLock(createInputs, ethers.ZeroHash, ethers.ZeroHash, "0x"),
      ).to.be.revertedWithCustomError(token, "InsufficientPartitionBalance");
    });

    it("should revert when creating a lock in the zero partition", async function () {
      const createInputs = encodeCreateLockInputs(
        ethers.ZeroHash,
        ethers.parseEther("100"),
        bobAddress,
      );

      await expect(
        token
          .connect(alice)
          .createLock(createInputs, ethers.ZeroHash, ethers.ZeroHash, "0x"),
      ).to.be.revertedWithCustomError(token, "InvalidPartition");
    });

    it("should revert when creating a zero-amount lock", async function () {
      const createInputs = encodeCreateLockInputs(PARTITION_A, 0n, bobAddress);

      await expect(
        token
          .connect(alice)
          .createLock(createInputs, ethers.ZeroHash, ethers.ZeroHash, "0x"),
      ).to.be.revertedWithCustomError(token, "InvalidAmount");
    });

    it("should prevent partition transfers when insufficient unlocked balance", async function () {
      const lockAmount = ethers.parseEther("800");

      await createTokenLock(
        token,
        alice,
        PARTITION_A,
        lockAmount,
        bobAddress,
      );

      // 200 unlocked, try to transfer 300 → revert
      await expect(
        token
          .connect(alice)
          .transferByPartition(
            PARTITION_A,
            bobAddress,
            ethers.parseEther("300"),
            "0x",
          ),
      ).to.be.revertedWithCustomError(token, "InsufficientPartitionBalance");

      // But should allow transfer within unlocked balance
      await token
        .connect(alice)
        .transferByPartition(
          PARTITION_A,
          bobAddress,
          ethers.parseEther("100"),
          "0x",
        );
      expect(
        await token.unlockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(ethers.parseEther("100"));
    });

    it("should keep locks in other partitions independent", async function () {
      await token
        .connect(owner)
        .issueByPartition(
          PARTITION_B,
          aliceAddress,
          ethers.parseEther("500"),
          "0x",
        );

      await createTokenLock(
        token,
        alice,
        PARTITION_A,
        ethers.parseEther("400"),
        bobAddress,
      );

      // Partition B unlocked balance is unaffected
      expect(
        await token.unlockedBalanceOfByPartition(PARTITION_B, aliceAddress),
      ).to.equal(ethers.parseEther("500"));
      // Partition A reflects the lock
      expect(
        await token.unlockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(ethers.parseEther("600"));

      // Can still freely move the whole of partition B
      await token
        .connect(alice)
        .transferByPartition(
          PARTITION_B,
          bobAddress,
          ethers.parseEther("500"),
          "0x",
        );
      expect(
        await token.balanceOfByPartition(PARTITION_B, bobAddress),
      ).to.equal(ethers.parseEther("500"));
    });
  });

  describe("redeemByPartition", function () {
    it("should burn unlocked tokens from a partition", async function () {
      await token
        .connect(alice)
        .redeemByPartition(PARTITION_A, ethers.parseEther("400"), "0x");

      expect(
        await token.balanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(ethers.parseEther("600"));
      expect(await token.balanceOf(aliceAddress)).to.equal(
        ethers.parseEther("600"),
      );
    });

    it("should emit RedeemedByPartition", async function () {
      await expect(
        token
          .connect(alice)
          .redeemByPartition(PARTITION_A, ethers.parseEther("100"), "0x"),
      )
        .to.emit(token, "RedeemedByPartition")
        .withArgs(
          PARTITION_A,
          aliceAddress,
          aliceAddress,
          ethers.parseEther("100"),
          "0x",
        );
    });

    it("should revert when redeeming locked tokens", async function () {
      await createTokenLock(
        token,
        alice,
        PARTITION_A,
        ethers.parseEther("800"),
        bobAddress,
      );

      // 200 unlocked, try to redeem 300 → revert
      await expect(
        token
          .connect(alice)
          .redeemByPartition(PARTITION_A, ethers.parseEther("300"), "0x"),
      ).to.be.revertedWithCustomError(token, "InsufficientPartitionBalance");

      // Redeeming within the unlocked balance still works
      await token
        .connect(alice)
        .redeemByPartition(PARTITION_A, ethers.parseEther("200"), "0x");
      expect(
        await token.unlockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(0);
    });

    it("should revert when redeeming from the zero partition", async function () {
      await expect(
        token
          .connect(alice)
          .redeemByPartition(ethers.ZeroHash, ethers.parseEther("1"), "0x"),
      ).to.be.revertedWithCustomError(token, "InvalidPartition");
    });
  });

  describe("canTransferByPartition status codes", function () {
    it("returns success when the partition has enough unlocked balance", async function () {
      const [code, , dest] = await token.canTransferByPartition(
        aliceAddress,
        bobAddress,
        PARTITION_A,
        ethers.parseEther("500"),
        "0x",
      );
      expect(code).to.equal(await token.TRANSFER_SUCCESS());
      expect(dest).to.equal(PARTITION_A);
    });

    it("returns insufficient-locked when the value exceeds the unlocked balance", async function () {
      await createTokenLock(
        token,
        alice,
        PARTITION_A,
        ethers.parseEther("800"),
        bobAddress,
      );

      const [code] = await token.canTransferByPartition(
        aliceAddress,
        bobAddress,
        PARTITION_A,
        ethers.parseEther("500"),
        "0x",
      );
      expect(code).to.equal(await token.TRANSFER_INSUFFICIENT_LOCKED());
    });

    it("returns insufficient-balance when the value exceeds the partition balance", async function () {
      const [code] = await token.canTransferByPartition(
        aliceAddress,
        bobAddress,
        PARTITION_A,
        ethers.parseEther("5000"),
        "0x",
      );
      expect(code).to.equal(await token.TRANSFER_INSUFFICIENT_BALANCE());
    });

    it("returns invalid-partition for the zero partition", async function () {
      const [code] = await token.canTransferByPartition(
        aliceAddress,
        bobAddress,
        ethers.ZeroHash,
        ethers.parseEther("1"),
        "0x",
      );
      expect(code).to.equal(await token.TRANSFER_INVALID_PARTITION());
    });
  });

  describe("Lock spending", function () {
    let lockId: string;

    beforeEach(async function () {
      const amount = ethers.parseEther("300");
      lockId = await createTokenLock(
        token,
        alice,
        PARTITION_A,
        amount,
        bobAddress,
      );
    });

    it("should spend lock and transfer within the same partition", async function () {
      const aliceBefore = await token.balanceOfByPartition(
        PARTITION_A,
        aliceAddress,
      );
      const bobBefore = await token.balanceOfByPartition(
        PARTITION_A,
        bobAddress,
      );

      await token.connect(alice).spendLock(lockId, "0x", "0x");

      // Partition transfer occurred
      expect(
        await token.balanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(aliceBefore - ethers.parseEther("300"));
      expect(
        await token.balanceOfByPartition(PARTITION_A, bobAddress),
      ).to.equal(bobBefore + ethers.parseEther("300"));
      expect(await token.balanceOf(bobAddress)).to.equal(
        ethers.parseEther("300"),
      );

      // Lock reservation released
      expect(
        await token.lockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(0);

      // Lock entry deleted
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
  });

  describe("Lock cancellation", function () {
    let lockId: string;

    beforeEach(async function () {
      const amount = ethers.parseEther("300");
      lockId = await createTokenLock(
        token,
        alice,
        PARTITION_A,
        amount,
        bobAddress,
      );
    });

    it("should cancel lock and restore unlocked partition balance", async function () {
      const aliceBalance = await token.balanceOfByPartition(
        PARTITION_A,
        aliceAddress,
      );

      await token.connect(alice).cancelLock(lockId, "0x", "0x");

      // No transfer happened
      expect(
        await token.balanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(aliceBalance);
      expect(
        await token.balanceOfByPartition(PARTITION_A, bobAddress),
      ).to.equal(0);

      // Locked balance released
      expect(
        await token.lockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(0);
      expect(
        await token.unlockedBalanceOfByPartition(PARTITION_A, aliceAddress),
      ).to.equal(aliceBalance);

      // Lock entry deleted
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
      const amount = ethers.parseEther("300");
      lockId = await createTokenLock(
        token,
        alice,
        PARTITION_A,
        amount,
        bobAddress,
      );
    });

    it("should delegate lock and let the delegate spend", async function () {
      await token
        .connect(alice)
        .delegateLock(lockId, "0x", charlieAddress, "0x");

      const lockInfo = await token.getLock(lockId);
      expect(lockInfo.spender).to.equal(charlieAddress);
      expect(lockInfo.owner).to.equal(aliceAddress); // Owner unchanged

      // Charlie can now spend the lock
      await token.connect(charlie).spendLock(lockId, "0x", "0x");

      // Tokens land in bob's partition A
      expect(
        await token.balanceOfByPartition(PARTITION_A, bobAddress),
      ).to.equal(ethers.parseEther("300"));
    });

    it("should revert if alice tries to spend after delegation", async function () {
      await token
        .connect(alice)
        .delegateLock(lockId, "0x", charlieAddress, "0x");

      await expect(
        token.connect(alice).spendLock(lockId, "0x", "0x"),
      ).to.be.revertedWithCustomError(token, "LockUnauthorized");
    });

    it("should revert if delegate tries to update the lock", async function () {
      await token
        .connect(alice)
        .delegateLock(lockId, "0x", charlieAddress, "0x");

      await expect(
        token
          .connect(charlie)
          .updateLock(
            lockId,
            "0x",
            ethers.keccak256(ethers.toUtf8Bytes("new spend hash")),
            ethers.ZeroHash,
            "0x",
          ),
      ).to.be.revertedWithCustomError(token, "LockImmutable");
    });
  });
});
