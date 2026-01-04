import { expect } from "chai";
import { randomBytes } from "crypto";
import { BytesLike, Signer, TypedDataEncoder } from "ethers";
import hre, { ethers } from "hardhat";
import { ILockableCapability, Noto, NotoFactory } from "../../../typechain-types";

export interface NotoLockOperation {
  txId: BytesLike;
  inputs: BytesLike[];
  outputs: BytesLike[];
  lockedOutputs: BytesLike[];
  proof: BytesLike;
}

export interface NotoUnlockOperation {
  txId: BytesLike;
  inputs: BytesLike[];
  outputs: BytesLike[];
  data: BytesLike;
  proof: BytesLike;
}

export interface NotoDelegateLockOperation {
  txId: BytesLike;
  inputs: BytesLike[];
  outputs: BytesLike[];
  proof: BytesLike;
}

export async function newUnlockHash(
  noto: Noto,
  txId: BytesLike,
  lockedInputs: string[],
  outputs: string[],
  data: string
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

export function createLockOptions(spendTxId: string) {
  const options = {
    spendTxId: spendTxId,
  };
  return ethers.AbiCoder.defaultAbiCoder().encode(
    ["tuple(bytes32)"],
    [[options.spendTxId]]
  );
}

export async function deployNotoInstance(
  notoFactory: NotoFactory,
  notary: string
) {
  const deployTx = await notoFactory.deploy(
    randomBytes32(),
    "NOTO",
    "NOTO",
    notary,
    "0x"
  );
  const deployReceipt = await deployTx.wait();
  const deployEvent = deployReceipt?.logs.find(
    (l) =>
      notoFactory.interface.parseLog(l)?.name ===
      "PaladinRegisterSmartContract_V0"
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
  data: string
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
  data: string
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

export function encodeLockParams(lockOp: NotoLockOperation): BytesLike {
  return ethers.AbiCoder.defaultAbiCoder().encode(
    ["tuple(bytes32,bytes32[],bytes32[],bytes32[],bytes)"], [
    [
      lockOp.txId,
      lockOp.inputs,
      lockOp.outputs,
      lockOp.lockedOutputs,
      lockOp.proof,
    ],
  ]);
}

export function encodeUnlockParams(unlockOp: NotoUnlockOperation): BytesLike {
  return ethers.AbiCoder.defaultAbiCoder().encode(
    ["tuple(bytes32,bytes32[],bytes32[],bytes,bytes)"], [
    [
      unlockOp.txId,
      unlockOp.inputs,
      unlockOp.outputs,
      unlockOp.data,
      unlockOp.proof,
    ]
  ])  
}

export function encodeDelegateLockParams(delegateOp: NotoDelegateLockOperation): BytesLike {
  return ethers.AbiCoder.defaultAbiCoder().encode(
    ["tuple(bytes32,bytes32[],bytes32[],bytes)"], [
    [
      delegateOp.txId,
      delegateOp.inputs,
      delegateOp.outputs,
      delegateOp.proof,
    ]
  ])  
}

export async function doLock(
  notary: Signer,
  noto: Noto,
  lockOp: NotoLockOperation,
  params: ILockableCapability.LockParamsStruct,
  data: string
) {
  const notaryAddr = await notary.getAddress();

  // NotoLockOperation
  const encodedParams = encodeLockParams(lockOp);

  const tx = await noto.connect(notary).createLock(encodedParams, params, data);
  const results = await tx.wait();
  expect(results).to.exist;
  const lockId = await noto.computeLockId(encodedParams);

  expect(results?.logs.length).to.equal(2);
 
  // First log is the ILockableCapability.LockUpdate standard event
  const event0 = noto.interface.parseLog(results!.logs[0]);
  expect(event0).to.exist;
  expect(event0?.name).to.equal("LockUpdated");
  expect(event0?.args.lockId).to.equal(lockId);
  expect(event0?.args.lock["owner"]).to.equal(notaryAddr);
  expect(event0?.args.lock["content"]).to.equal(ethers.AbiCoder.defaultAbiCoder().encode(["bytes32[]"],[lockOp.lockedOutputs]));
  expect(event0?.args.lock["spender"]).to.equal(notaryAddr);
  expect(event0?.args.lock["spendHash"]).to.equal(params.spendHash);
  expect(event0?.args.lock["cancelHash"]).to.equal(params.cancelHash);
  expect(event0?.args.lock["options"]).to.equal(params.options);
  expect(event0?.args.data).to.equal(data);

  // Second log is the INoto.NotoLockCreated event that gives the inputs and outputs
  const event1 = noto.interface.parseLog(results!.logs[1]);
  expect(event1).to.exist;
  expect(event1?.name).to.equal("NotoLockCreated");
  expect(event1?.args.inputs).to.deep.equal(lockOp.inputs);
  expect(event1?.args.outputs).to.deep.equal(lockOp.outputs);
  expect(event1?.args.lockedOutputs).to.deep.equal(lockOp.lockedOutputs);
  expect(event1?.args.proof).to.deep.equal(lockOp.proof);
  expect(event1?.args.data).to.equal(data);

  for (const input of lockOp.inputs) {
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of lockOp.outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
  for (const output of lockOp.lockedOutputs) {
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
  lockedInputs: string[],
  outputs: string[],
  data: string
) {
  const encodedParams = encodeUnlockParams({
    txId,
    inputs: lockedInputs,
    outputs,
    data,
    proof: '0x',
  });
  const outerData = randomBytes32();
  const tx = await noto.connect(sender).spendLock(lockId, encodedParams, outerData);
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
  expect(event1?.args.proof).to.equal('0x');
  expect(event1?.args.txData).to.equal(data);
  expect(event1?.args.data).to.equal(outerData);

  for (const input of lockedInputs) {
    expect(await noto.getLockId(input)).to.equal(
      "0x0000000000000000000000000000000000000000000000000000000000000000"
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
  spendHash: string,
  cancelHash: string,
  data: string,
) {
  const notaryAddr = await notary.getAddress();

  const encodedParams = encodeLockParams({
    txId,
    inputs: [],
    outputs: [],
    lockedOutputs: [],
    proof: '0x',
  });

  const options = createLockOptions(spendTxId);
  const params: ILockableCapability.LockParamsStruct = {
    spendHash: spendHash,
    cancelHash: cancelHash,
    options: options,
  };
  const tx = await noto.connect(notary).updateLock(lockId, encodedParams, params, data);
  const results = await tx.wait();
  expect(results).to.exist;

  // First log is the ILockableCapability.LockUpdate standard event
  const event0 = noto.interface.parseLog(results!.logs[0]);
  expect(event0).to.exist;
  expect(event0?.name).to.equal("LockUpdated");
  expect(event0?.args.lockId).to.equal(lockId);
  expect(event0?.args.lock["owner"]).to.equal(notaryAddr);
  expect(event0?.args.lock["spender"]).to.equal(notaryAddr);
  expect(event0?.args.lock["spendHash"]).to.equal(params.spendHash);
  expect(event0?.args.lock["cancelHash"]).to.equal(params.cancelHash);
  expect(event0?.args.lock["options"]).to.equal(params.options);

  // Second log is the INoto.NotoLockUpdated event that gives the inputs and outputs
  const event1 = noto.interface.parseLog(results!.logs[1]);
  expect(event1).to.exist;
  expect(event1?.name).to.equal("NotoLockUpdated");
  expect(event1?.args.inputs).to.deep.equal([]);
  expect(event1?.args.outputs).to.deep.equal([]);
  expect(event1?.args.proof).to.deep.equal("0x");
  expect(event1?.args.data).to.equal(data);

}

export async function doDelegateLock(
  txId: string,
  notary: Signer,
  noto: Noto,
  lockId: string,
  delegate: string,
  data: string,
) {
  const delegateLockParams = {
    txId: txId,
    inputs: [],
    outputs: [],
    proof: '0x',
  };
  // NotoDelegateOperation
  const encodedParams = encodeDelegateLockParams(delegateLockParams)
  const tx = await noto.connect(notary).delegateLock(lockId, encodedParams, delegate, data);
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