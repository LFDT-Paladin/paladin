import { loadFixture } from "@nomicfoundation/hardhat-toolbox/network-helpers";
import { expect } from "chai";
import { ethers } from "hardhat";
import { Merkletree, InMemoryDB, str2Bytes, HashAlgorithm, Hash } from "@iden3/js-merkletree";
import { Noto, NotoNullifiers } from "../../../typechain-types";
import {
  deployNotoFactory,
  registerNotoNullifiersImplementation,
  deployNotoInstance,
  doDelegateLock,
  doLockWithNullifiers,
  doMint,
  doPrepareUnlock,
  doTransferWithNullifiers,
  doUnlock,
  newUnlockHash,
  newUTXO,
  randomBytes32,
  UTXO,
  NotoLockOperation
} from "./util";
import { randomBytes } from "crypto";

describe("NotoNullifiers", function () {
  async function deployNotoFixture() {
    const [notary, other] = await ethers.getSigners();

    const notoFactory = await deployNotoFactory();
    const { smtLib } = await registerNotoNullifiersImplementation(notoFactory);
    const Noto = await ethers.getContractFactory("NotoNullifiers", {
      libraries: {
        SmtLib: smtLib.target,
      },
    });
    const noto = Noto.attach(
      await deployNotoInstance(notoFactory, notary.address, "nullifiers")
    );

    return { noto: noto as NotoNullifiers, notary, other };
  }

  describe("UTXO lifecycle and double-spend protections", function () {
    let noto: NotoNullifiers;
    let notary: any;
    let smtNotary: Merkletree;
    let txo1: UTXO;
    let txo2: UTXO;
    let txo3: UTXO;
    let txo4: UTXO;

    before(async function () {
      ({ noto, notary } = await loadFixture(deployNotoFixture));
      const storage1 = new InMemoryDB(str2Bytes(""), HashAlgorithm.Keccak256);
      smtNotary = new Merkletree(storage1, true, 64);
    });

    it("mint UTXOs", async function () {
      txo1 = newUTXO(10);
      txo2 = newUTXO(20);
      txo3 = newUTXO(10);
      txo4 = newUTXO(20);

      // Make two UTXOs
      await doMint(
        randomBytes32(),
        notary,
        noto as unknown as Noto,
        [txo1.hash!, txo2.hash!],
        randomBytes32(),
        true
      );
      const hash1 = BigInt(txo1.hash!);
      const hash2 = BigInt(txo2.hash!);
      await smtNotary.add(hash1, hash1);
      await smtNotary.add(hash2, hash2);
    });

    it("Check for double-mint protection", async function () {
      await expect(
        doMint(randomBytes32(), notary, noto as unknown as Noto, [txo1.hash!], randomBytes32(), true)
      ).rejectedWith("NotoInvalidOutput");
    });

    it("Check for spend unknown root protection", async function () {
      const root = await smtNotary.root();
      await expect(
        doTransferWithNullifiers(randomBytes32(), notary, noto, [txo1.nullifier!], [txo3.hash!], (root.bigInt() + 1n).toString(10), randomBytes32())
      ).rejectedWith("NotoInvalidRoot");
    });

    it("Check for double-spend protection", async function () {
      // Spend txo1, output txo3
      const root = await smtNotary.root();
      await doTransferWithNullifiers(
        randomBytes32(),
        notary,
        noto,
        [txo1.nullifier!],
        [txo3.hash!],
        root.bigInt().toString(10),
        randomBytes32()
      );
      await smtNotary.add(BigInt(txo3.hash!), BigInt(txo3.hash!));

      // attempting to spend again
      await expect(
        doTransferWithNullifiers(randomBytes32(), notary, noto, [txo1.nullifier!], [txo3.hash!], root.bigInt().toString(10), randomBytes32())
      ).rejectedWith("NotoInvalidInput");
    });

    it("Spend another", async function () {
      // Spend another
      const root1 = await smtNotary.root();
      await doTransferWithNullifiers(
        randomBytes32(),
        notary,
        noto,
        [txo2.nullifier!],
        [txo4.hash!],
        root1.bigInt().toString(10),
        randomBytes32()
      );
      await smtNotary.add(BigInt(txo4.hash!), BigInt(txo4.hash!));

      // Spend the last one
      const root2 = await smtNotary.root();
      const _txo1 = newUTXO(10);
      await doTransferWithNullifiers(
        randomBytes32(),
        notary,
        noto,
        [txo3.nullifier!],
        [_txo1.hash!],
        root2.bigInt().toString(10),
        randomBytes32()
      );
    });
  });

  describe("lock and unlock lifecycle and protections", function () {
    let noto: NotoNullifiers;
    let notary: any;
    let smtNotary: Merkletree;
    let delegate: any;
    let other: any;

    before(async function () {
      const [_, s1, s2] = await ethers.getSigners();
      delegate = s1;
      other = s2;
    });

    describe("lock and unlock", function () {
      let txo1: UTXO;
      let txo2: UTXO;
      let txo3: UTXO;
      let txo4: UTXO;
      let locked1: UTXO;
      let locked2: UTXO;
      let lockId: string;

      before(async function () {
        ({ noto, notary } = await loadFixture(deployNotoFixture));
        const storage1 = new InMemoryDB(str2Bytes(""), HashAlgorithm.Keccak256);
        smtNotary = new Merkletree(storage1, true, 64);
      });

      it("mint UTXOs", async function () {
        txo1 = newUTXO(20);
        txo2 = newUTXO(30);

        // Make two UTXOs
        await doMint(
          randomBytes32(),
          notary,
          noto as unknown as Noto,
          [txo1.hash!, txo2.hash!],
          randomBytes32(),
          true
        );
        await smtNotary.add(BigInt(txo1.hash!), BigInt(txo1.hash!));
        await smtNotary.add(BigInt(txo2.hash!), BigInt(txo2.hash!));
      });

      it("lock UTXOs", async function () {
        // Lock both of them
        const root = await smtNotary.root();
        txo3 = newUTXO(15);
        locked1 = newUTXO(35);
        lockId = await doLockWithNullifiers(
          randomBytes32(),
          notary,
          noto,
          [txo1.nullifier!, txo2.nullifier!],
          [txo3.hash!],
          [locked1.hash!],
          root.bigInt().toString(10),
          randomBytes32()
        );
        // only the outputs are tracked in the merkle tree.
        // the locked outputs are tracked in the locked states map and
        // to be consumed using the UTXO ids instead of nullifiers
        await smtNotary.add(BigInt(txo3.hash!), BigInt(txo3.hash!));
      });

      it("Check that the same state cannot be locked again", async function () {
        // this root should fail the merkle proof check at the notary, because
        // the "locked1" UTXO is not in the merkle tree (never added to the tree)
        const root = await smtNotary.root();
        await expect(
          doLockWithNullifiers(randomBytes32(), notary, noto, [], [], [locked1.hash!], root.bigInt().toString(10), randomBytes32())
        ).to.be.rejectedWith("NotoInvalidOutput");
      });

      it("Check that locked value cannot be spent", async function () {
        // when a sender attempts to spend a locked UTXO, the notary
        // is expected to reject it because the merkle proof check will fail
        const { proof } = await smtNotary.generateProof(BigInt(locked1.hash!));
        // the "false" existence flag would cause the notary to reject the transaction
        expect(proof.existence).to.be.false;
      });

      it("unlock UTXOs", async function () {
        // Unlock the UTXO
        txo4 = newUTXO(35);
        await doUnlock(
          randomBytes32(),
          notary,
          noto as unknown as Noto,
          lockId,
          [locked1.hash!],
          [txo4.hash!],
          randomBytes32(),
          true
        );
        await smtNotary.add(BigInt(txo4.hash!), BigInt(txo4.hash!));
      });

      it("Check that the same state cannot be unlocked again", async function () {
        await expect(
          doUnlock(
            randomBytes32(),
            notary,
            noto as unknown as Noto,
            lockId,
            [locked1.hash!],
            [],
            randomBytes32(),
            true
          )
        ).to.be.rejectedWith("LockNotActive");
      });

      it("lock more UTXOs", async function () {
        const root = await smtNotary.root();
        locked2 = newUTXO(35);
        lockId = await doLockWithNullifiers(
          randomBytes32(),
          notary,
          noto,
          [txo4.nullifier!],
          [],
          [locked2.hash!],
          root.bigInt().toString(10),
          randomBytes32()
        );
      });

      it("prepare unlock, delegate lock, and perform unlock", async function () {
        const txo5 = newUTXO(35);
        const txo6 = newUTXO(35);

        // Prepare unlock operations (both spend and cancel) before unlocking
        const unlockTxId = randomBytes32();
        const unlockData = randomBytes32();
        const spendHash = await newUnlockHash(
          noto,
          unlockTxId,
          [locked2.hash!],
          [txo5.hash!],
          unlockData
        );
        const cancelHash = await newUnlockHash(noto, unlockTxId, [locked2.hash!], [txo6.hash!], unlockData);
        await doPrepareUnlock(
          randomBytes32(),
          notary,
          noto,
          lockId,
          unlockTxId,
          spendHash,
          cancelHash,
          unlockData,
        );

        // Delegate the unlock
        await doDelegateLock(
          randomBytes32(),
          notary,
          noto as unknown as Noto,
          lockId,
          delegate.address,
          randomBytes32()
        );

        // Attempt to perform an incorrect unlock
        await expect(
          doUnlock(
            unlockTxId,
            delegate,
            noto as unknown as Noto,
            lockId,
            [locked2.hash!],
            [txo5.hash!],
            randomBytes32(),
            true
          ) // wrong data
        ).to.be.rejectedWith("NotoInvalidUnlockHash");

        await expect(
          doUnlock(randomBytes32(), other, noto as unknown as Noto, lockId, [locked2.hash!], [txo5.hash!], unlockData, true) // wrong delegate
        ).to.be.rejectedWith("LockUnauthorized");

        // Perform the prepared unlock
        await doUnlock(
          unlockTxId,
          delegate,
          noto as unknown as Noto,
          lockId,
          [locked2.hash!],
          [txo5.hash!],
          unlockData,
          true
        );
      });
    });

    describe("Duplicate TXID reverts transfer", () => {
      before(async function () {
        ({ noto, notary } = await loadFixture(deployNotoFixture));
      });

      it("should fail", async function () {
        const txo1 = newUTXO(1);
        const txo2 = newUTXO(2);
        const txo3 = newUTXO(3);
        const txo4 = newUTXO(4);
        const txId1 = randomBytes32();

        // Make two UTXOs - should succeed
        await expect(doMint(txId1, notary, noto as unknown as Noto, [txo1.hash!, txo2.hash!], randomBytes32(), true)).to.be.fulfilled;

        // Make two more UTXOs with the same TX ID - should fail
        const root = await smtNotary.root();
        await expect(
          doTransferWithNullifiers(txId1, notary, noto, [], [txo3.hash!, txo4.hash!], root.bigInt().toString(10), randomBytes32())
        ).rejectedWith("NotoDuplicateTransaction");
      });
    });

    describe("Duplicate TXID reverts lock, unlock, prepare unlock, and delegate lock", () => {
      let txo1: UTXO;
      let txo2: UTXO;
      let txo3: UTXO;
      let txo4: UTXO;
      let locked1: UTXO;
      let locked2: UTXO;
      let txId1: string;
      let lockId: string;

      before(async function () {
        ({ noto, notary } = await loadFixture(deployNotoFixture));
        const storage1 = new InMemoryDB(str2Bytes(""), HashAlgorithm.Keccak256);
        smtNotary = new Merkletree(storage1, true, 64);
      });

      it("mint UTXOs", async function () {
        txo1 = newUTXO(1);
        txo2 = newUTXO(2);
        txId1 = randomBytes32();

        // Make two UTXOs
        await expect(doMint(txId1, notary, noto as unknown as Noto, [txo1.hash!, txo2.hash!], randomBytes32(), true)).to.be.fulfilled;
        await smtNotary.add(BigInt(txo1.hash!), BigInt(txo1.hash!));
        await smtNotary.add(BigInt(txo2.hash!), BigInt(txo2.hash!));
      });

      it("should fail on duplicate txId", async function () {
        txo3 = newUTXO(1);
        locked1 = newUTXO(2);
        // Try to lock both of them using the same TX ID - should fail
        const root = await smtNotary.root();
        await expect(
          doLockWithNullifiers(
            txId1,
            notary,
            noto,
            [txo1.nullifier!, txo2.nullifier!],
            [txo3.hash!],
            [locked1.hash!],
            root.bigInt().toString(10),
            randomBytes32()
          )
        ).to.be.rejectedWith("NotoDuplicateTransaction");
      });

      it("should fail on duplicate txId during unlock", async function () {
        // Lock both of them using a new TX ID - should succeed
        const root = await smtNotary.root();
        lockId = await doLockWithNullifiers(
          randomBytes32(),
          notary,
          noto,
          [txo1.nullifier!, txo2.nullifier!],
          [txo3.hash!],
          [locked1.hash!],
          root.bigInt().toString(10),
          randomBytes32()
        );

        // Unlock the UTXO using the same TX ID as the transfer - should fail
        locked2 = newUTXO(1);
        txo4 = newUTXO(1);
        await expect(
          doUnlock(
            txId1,
            notary,
            noto as unknown as Noto,
            lockId,
            [locked1.hash!], // unlock inputs are locked outputs which are identified by their hashes (not nullifiers)
            [txo4.hash!],
            randomBytes32(),
            true
          )
        ).to.be.rejectedWith("NotoDuplicateTransaction");
      });

      it("should fail on duplicate txId during prepare unlock and delegate lock", async function () {
        const txo5 = newUTXO(1);
        const txo6 = newUTXO(1);

        // Prepare an unlock operation
        const unlockData = randomBytes32();
        const unlockTxId = randomBytes32();
        const spendHash = await newUnlockHash(
          noto as unknown as Noto,
          unlockTxId,
          [locked2.hash!],
          [txo5.hash!],
          unlockData
        );
        const cancelHash = await newUnlockHash(noto, unlockTxId, [locked2.hash!], [txo6.hash!], unlockData);
        await doPrepareUnlock(
          randomBytes32(),
          notary,
          noto,
          lockId,
          unlockTxId,
          spendHash,
          cancelHash,
          unlockData,
        );

        // Delegate the lock using the same TX ID as the transfer - should fail
        await expect(
          doDelegateLock(
            txId1,
            notary,
            noto as unknown as Noto,
            lockId,
            "0x" + Buffer.from(randomBytes(20)).toString("hex"),
            randomBytes32(),
          )
        ).to.be.rejectedWith("NotoDuplicateTransaction");
      });
    });
  });
});
