import { expect } from "chai";
import { randomBytes } from "crypto";
import { BytesLike, Signer, TypedDataEncoder } from "ethers";
import hre, { ethers } from "hardhat";
import { Noto, NotoFactory } from "../../../typechain-types";

export interface NotoLockOptions {
  spendTxId: BytesLike;
}

export interface NotoCreateLockArgs {
  txId: BytesLike;
  inputs: BytesLike[];
  outputs: BytesLike[];
  contents: BytesLike[];
  newLockState: BytesLike;
  options: NotoLockOptions;
  proof: BytesLike;
}

export interface NotoUpdateLockArgs {
  txId: BytesLike;
  oldLockState: BytesLike;
  newLockState: BytesLike;
  options: NotoLockOptions;
  proof: BytesLike;
}

export interface NotoSpendLockArgs {
  txId: BytesLike;
  inputs: BytesLike[];
  outputs: BytesLike[];
  data: BytesLike;
  proof: BytesLike;
}

export interface NotoDelegateLockArgs {
  txId: BytesLike;
  oldLockState: BytesLike;
  newLockState: BytesLike;
  proof: BytesLike;
}

export async function newUnlockHash(
  noto: Noto,
  txId: BytesLike,
  lockedInputs: string[],
  outputs: string[],
  data: string,
) {
  const domain = {
    name: "noto",
    version: "0.0.1",
    chainId: hre.network.config.chainId,
    verifyingContract: await noto.getAddress(),
  };
  const types = {
    Unlock: [
      { name: "txId", type: "bytes32" },
      { name: "lockedInputs", type: "bytes32[]" },
      { name: "outputs", type: "bytes32[]" },
      { name: "data", type: "bytes" },
    ],
  };
  const value = { txId, lockedInputs, outputs, data };
  return TypedDataEncoder.hash(domain, types, value);
}

export function randomBytes32() {
  return "0x" + Buffer.from(randomBytes(32)).toString("hex");
}

export function fakeTXO() {
  return randomBytes32();
}

export async function deployNotoInstance(
  notoFactory: NotoFactory,
  notary: string,
) {
  const deployTx = await notoFactory.deploy(
    randomBytes32(),
    "NOTO",
    "NOTO",
    notary,
    "0x",
  );
  const deployReceipt = await deployTx.wait();
  const deployEvent = deployReceipt?.logs.find(
    (l) =>
      notoFactory.interface.parseLog(l)?.name ===
      "PaladinRegisterSmartContract_V0",
  );
  expect(deployEvent).to.exist;
  return deployEvent && "args" in deployEvent ? deployEvent.args.instance : "";
}

export async function doTransfer(
  txId: string,
  notary: Signer,
  noto: Noto,
  inputs: string[],
  outputs: string[],
  data: string,
) {
  const tx = await noto
    .connect(notary)
    .transfer(txId, inputs, outputs, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("Transfer");
    expect(event?.args.inputs).to.deep.equal(inputs);
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.data).to.deep.equal(data);
  }
  for (const input of inputs) {
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
}

export async function doMint(
  txId: string,
  notary: Signer,
  noto: Noto,
  outputs: string[],
  data: string,
) {
  const tx = await noto.connect(notary).transfer(txId, [], outputs, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("Transfer");
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.data).to.deep.equal(data);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
}

export function encodeCreateLockArgs(
  lockOp: NotoCreateLockArgs,
): BytesLike {
  return ethers.AbiCoder.defaultAbiCoder().encode(
    [
      "tuple(bytes32,bytes32[],bytes32[],bytes32[],bytes32,tuple(bytes32),bytes)",
    ],
    [
      [
        lockOp.txId,
        lockOp.inputs,
        lockOp.outputs,
        lockOp.contents,
        lockOp.newLockState,
        [lockOp.options.spendTxId],
        lockOp.proof,
      ],
    ],
  );
}

export function encodeUpdateLockArgs(
  lockOp: NotoUpdateLockArgs,
): BytesLike {
  return ethers.AbiCoder.defaultAbiCoder().encode(
    ["tuple(bytes32,bytes32,bytes32,tuple(bytes32),bytes)"],
    [
      [
        lockOp.txId,
        lockOp.oldLockState,
        lockOp.newLockState,
        [lockOp.options.spendTxId],
        lockOp.proof,
      ],
    ],
  );
}

export function encodeUnlockArgs(unlockOp: NotoSpendLockArgs): BytesLike {
  return ethers.AbiCoder.defaultAbiCoder().encode(
    ["tuple(bytes32,bytes32[],bytes32[],bytes,bytes)"],
    [
      [
        unlockOp.txId,
        unlockOp.inputs,
        unlockOp.outputs,
        unlockOp.data,
        unlockOp.proof,
      ],
    ],
  );
}

export function encodeDelegateLockArgs(
  delegateOp: NotoDelegateLockArgs,
): BytesLike {
  return ethers.AbiCoder.defaultAbiCoder().encode(
    ["tuple(bytes32,bytes32,bytes32,bytes)"],
    [
      [
        delegateOp.txId,
        delegateOp.oldLockState,
        delegateOp.newLockState,
        delegateOp.proof,
      ],
    ],
  );
}

export async function doLock(
  notary: Signer,
  noto: Noto,
  lockOp: NotoCreateLockArgs,
  spendCommitment: BytesLike,
  cancelCommitment: BytesLike,
  data: string,
) {
  const notaryAddr = await notary.getAddress();
  const encodedParams = encodeCreateLockArgs(lockOp);

  const tx = await noto
    .connect(notary)
    .createLock(encodedParams, spendCommitment, cancelCommitment, data);
  const results = await tx.wait();
  expect(results).to.exist;
  const lockId = await noto.computeLockId(encodedParams);

  expect(results?.logs.length).to.equal(2);

  // First log is the ILockableCapability.LockCreated standard event
  const event0 = noto.interface.parseLog(results!.logs[0]);
  expect(event0).to.exist;
  expect(event0?.name).to.equal("LockCreated");
  expect(event0?.args.lockId).to.equal(lockId);
  expect(event0?.args.owner).to.equal(notaryAddr);
  expect(event0?.args.spender).to.equal(notaryAddr);
  expect(event0?.args.data).to.equal(data);

  // Second log is the INoto.NotoLockCreated event that gives the inputs and outputs
  const event1 = noto.interface.parseLog(results!.logs[1]);
  expect(event1).to.exist;
  expect(event1?.name).to.equal("NotoLockCreated");
  expect(event1?.args.inputs).to.deep.equal(lockOp.inputs);
  expect(event1?.args.outputs).to.deep.equal(lockOp.outputs);
  expect(event1?.args.contents).to.deep.equal(lockOp.contents);
  expect(event1?.args.proof).to.deep.equal(lockOp.proof);
  expect(event1?.args.data).to.equal(data);

  for (const input of lockOp.inputs) {
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of lockOp.outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
  for (const output of lockOp.contents) {
    expect(await noto.getLockId(output)).to.equal(lockId);
    expect(await noto.isUnspent(output)).to.equal(false);
  }
  return lockId;
}

export async function doUnlock(
  txId: string,
  sender: Signer,
  noto: Noto,
  lockId: string,
  oldLockStateId: string,
  lockedInputs: string[],
  outputs: string[],
  data: string,
) {
  const encodedParams = encodeUnlockArgs({
    txId,
    inputs: lockedInputs,
    outputs,
    data,
    proof: "0x",
  });
  const outerData = randomBytes32();
  const tx = await noto
    .connect(sender)
    .spendLock(lockId, encodedParams, outerData);
  const results = await tx.wait();
  expect(results).to.exist;

  expect(results?.logs.length).to.equal(2);

  const event0 = noto.interface.parseLog(results!.logs[0]);
  expect(event0).to.exist;
  expect(event0?.name).to.equal("LockSpent");
  expect(event0?.args.lockId).to.equal(lockId);
  expect(event0?.args.spender).to.equal(await sender.getAddress());
  expect(event0?.args.data).to.equal(outerData);

  const event1 = noto.interface.parseLog(results!.logs[1]);
  expect(event1).to.exist;
  expect(event1?.name).to.equal("NotoLockSpent");
  expect(event1?.args.txId).to.equal(txId);
  expect(event1?.args.lockId).to.equal(lockId);
  expect(event1?.args.spender).to.equal(await sender.getAddress());
  expect(event1?.args.inputs).to.deep.equal(lockedInputs);
  expect(event1?.args.outputs).to.deep.equal(outputs);
  expect(event1?.args.oldLockState).to.deep.equal(oldLockStateId);
  expect(event1?.args.proof).to.equal("0x");
  expect(event1?.args.txData).to.equal(data);
  expect(event1?.args.data).to.equal(outerData);

  for (const input of lockedInputs) {
    expect(await noto.getLockId(input)).to.equal(
      "0x0000000000000000000000000000000000000000000000000000000000000000",
    );
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
}

export async function doPrepareUnlock(
  txId: string,
  notary: Signer,
  noto: Noto,
  lockId: string,
  spendTxId: string,
  oldLockStateId: string,
  newLockStateId: string,
  spendHash: string,
  cancelHash: string,
  data: string,
) {
  const notaryAddr = await notary.getAddress();

  const encodedParams = encodeUpdateLockArgs({
    txId,
    oldLockState: oldLockStateId,
    newLockState: newLockStateId,
    options: { spendTxId },
    proof: "0x",
  });

  const tx = await noto
    .connect(notary)
    .updateLock(lockId, encodedParams, spendHash, cancelHash, data);
  const results = await tx.wait();
  expect(results).to.exist;

  // First log is the ILockableCapability.LockUpdate standard event
  const event0 = noto.interface.parseLog(results!.logs[0]);
  expect(event0).to.exist;
  expect(event0?.name).to.equal("LockUpdated");
  expect(event0?.args.lockId).to.equal(lockId);
  expect(event0?.args.operator).to.equal(notaryAddr);
  expect(event0?.args.data).to.equal(data);

  // Second log is the INoto.NotoLockUpdated event that gives the inputs and outputs
  const event1 = noto.interface.parseLog(results!.logs[1]);
  expect(event1).to.exist;
  expect(event1?.name).to.equal("NotoLockUpdated");
  expect(event1?.args.oldLockState).to.deep.equal(oldLockStateId);
  expect(event1?.args.newLockState).to.deep.equal(newLockStateId);
  expect(event1?.args.proof).to.deep.equal("0x");
  expect(event1?.args.data).to.equal(data);
}

export async function doDelegateLock(
  txId: string,
  notary: Signer,
  noto: Noto,
  lockId: string,
  oldLockStateId: string,
  newLockStateId: string,
  delegate: string,
  data: string,
) {
  const delegateLockParams = {
    txId: txId,
    oldLockState: oldLockStateId,
    newLockState: newLockStateId,
    proof: "0x",
  };
  // NotoDelegateOperation
  const encodedParams = encodeDelegateLockArgs(delegateLockParams);
  const tx = await noto
    .connect(notary)
    .delegateLock(lockId, encodedParams, delegate, data);
  const results = await tx.wait();
  expect(results).to.exist;

  // First log is the ILockableCapability.LockDelegated standard event
  const event0 = noto.interface.parseLog(results!.logs[0]);
  expect(event0).to.exist;
  expect(event0?.name).to.equal("LockDelegated");
  expect(event0?.args.lockId).to.equal(lockId);
  expect(event0?.args.from).to.equal(await notary.getAddress());
  expect(event0?.args.to).to.equal(delegate);
  expect(event0?.args.data).to.equal(data);

  // Second log is the INoto.NotoLockDelegated event that gives the inputs and outputs
  const event1 = noto.interface.parseLog(results!.logs[1]);
  expect(event1).to.exist;
  expect(event1?.name).to.equal("NotoLockDelegated");
  expect(event1?.args.txId).to.equal(txId);
  expect(event1?.args.lockId).to.equal(lockId);
  expect(event1?.args.from).to.equal(await notary.getAddress());
  expect(event1?.args.to).to.equal(delegate);
  expect(event1?.args.proof).to.deep.equal("0x");
  expect(event1?.args.data).to.equal(data);

  const lockInfo = await noto.getLock(lockId);
  expect(lockInfo.spender).to.equal(delegate);
}
