import { expect } from "chai";
import { randomBytes } from "crypto";
import { Signer, TypedDataEncoder } from "ethers";
import hre, { ethers } from "hardhat";
import { Artifact } from "hardhat/types";
import { poseidonContract } from "circomlibjs";

import { Noto, NotoFactory } from "../../../typechain-types";
import { NotoNullifiers } from "../../../typechain-types/contracts/domains/noto";

export async function newUnlockHash(
  noto: Noto,
  lockedInputs: string[],
  lockedOutputs: string[],
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
      { name: "lockedInputs", type: "bytes32[]" },
      { name: "lockedOutputs", type: "bytes32[]" },
      { name: "outputs", type: "bytes32[]" },
      { name: "data", type: "bytes" },
    ],
  };
  const value = { lockedInputs, lockedOutputs, outputs, data };
  return TypedDataEncoder.hash(domain, types, value);
}

export function randomBytes32() {
  return "0x" + Buffer.from(randomBytes(32)).toString("hex");
}

export function fakeTXO() {
  return randomBytes32();
}

export interface UTXO {
  amount: number;
  salt: string; // bytes32
  owner: string; // address
  hash?: string; // bytes32
  nullifier?: string; // bytes32
}

export function newUTXO(amount: number): UTXO {
  const salt = randomBytes32();
  const ownerAddress = "0x" + Buffer.from(randomBytes(20)).toString("hex");
  const coin = {
    amount,
    salt,
    owner: ownerAddress,
  } as UTXO;
  coin.hash = eip712Hash(coin);
  coin.nullifier = eip712Nullifier(coin);
  return coin;
}

export function eip712Hash(coin: UTXO): string {
  const types = {
    notoCoin: [
      { name: "amount", type: "uint256" },
      { name: "salt", type: "bytes32" },
      { name: "owner", type: "address" },
    ],
  };

  const message = {
    owner: coin.owner,
    amount: coin.amount,
    salt: coin.salt,
  };

  const structHash = ethers.TypedDataEncoder.hashStruct("notoCoin", types, message);
  return structHash;
}

export function eip712Nullifier(coin: UTXO): string {
  const types = {
    notoNullifier: [
      { name: "amount", type: "uint256" },
      { name: "salt", type: "bytes32" },
    ],
  };
  const message = {
    amount: coin.amount,
    salt: coin.salt,
  };

  const structHash = ethers.TypedDataEncoder.hashStruct(
    "notoNullifier",
    types,
    message
  );
  return structHash;
}

export async function deployNotoFactory(): Promise<any> {
  const NotoFactory = await ethers.getContractFactory("NotoFactory");
  const notoFactory = await NotoFactory.deploy();
  return notoFactory;
}

export async function registerNotoNullifiersImplementation(
  notoFactory: NotoFactory
) {
  const [deployer] = await ethers.getSigners();
  // deploy SmtLib library
  const SmtLibFactory = await ethers.getContractFactory("SmtLib");
  const smtLib = await SmtLibFactory.deploy();

  // deploy NotoNullifiers implementation
  const NotoNullifiersFactory = await ethers.getContractFactory("NotoNullifiers", {
    libraries: {
      SmtLib: smtLib.target,
    },
  });
  const notoNullifiersImpl = await NotoNullifiersFactory.deploy();

  // register the implementation in the factory
  const tx = await notoFactory
    .connect(deployer)
    .registerImplementation("nullifiers", notoNullifiersImpl.target);
  await tx.wait();

  return { smtLib };
}

export async function deployNotoInstance(
  notoFactory: NotoFactory,
  notary: string,
  implName: string = "default"
) {
  const deployTx = await notoFactory.deploy(
    implName,
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
    expect(event?.name).to.equal("NotoTransfer");
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

export async function doTransferWithNullifiers(
  txId: string,
  notary: Signer,
  noto: NotoNullifiers,
  nullifiers: string[],
  outputs: string[],
  root: string,
  data: string
) {
  // build the nullifiers for the inputs

  const tx = await noto
    .connect(notary)["transfer(bytes32,bytes32[],bytes32[],uint256,bytes,bytes)"](txId, nullifiers, outputs, root, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoTransfer");
    expect(event?.args.inputs).to.deep.equal(nullifiers);
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.data).to.deep.equal(data);
  }

  // TODO: may need updates if the function is modified to
  // support "spent/unspent/unknown" states
  for (const input of nullifiers) {
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(false);
  }
}

export async function doMint(
  txId: string,
  notary: Signer,
  noto: Noto,
  outputs: string[],
  data: string,
  asNullifiers: boolean = false
) {
  const tx = await noto.connect(notary).mint(txId, outputs, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoTransfer");
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.data).to.deep.equal(data);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(asNullifiers ? false : true);
  }
}

export async function doLock(
  txId: string,
  notary: Signer,
  noto: Noto,
  inputs: string[],
  outputs: string[],
  lockedOutputs: string[],
  data: string
): Promise<string> {
  const tx = await noto
    .connect(notary)
    .lock(txId, inputs, outputs, lockedOutputs, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  let lockId = "";
  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoLock");
    expect(event?.args.inputs).to.deep.equal(inputs);
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.lockedOutputs).to.deep.equal(lockedOutputs);
    expect(event?.args.data).to.deep.equal(data);
    if (event?.args.lockId) {
      lockId = event.args.lockId;
    }
  }
  for (const input of inputs) {
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(true);
  }
  for (const output of lockedOutputs) {
    expect(await noto.isLocked(output)).to.equal(true);
    expect(await noto.isUnspent(output)).to.equal(false);
  }
  return lockId;
}

export async function doLockWithNullifiers(
  txId: string,
  notary: Signer,
  noto: NotoNullifiers,
  nullifiers: string[],
  outputs: string[],
  lockedOutputs: string[],
  root: string,
  data: string
): Promise<string> {
  const tx = await noto
    .connect(notary)["lock(bytes32,bytes32[],bytes32[],bytes32[],uint256,bytes,bytes)"](txId, nullifiers, outputs, lockedOutputs, root, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  let lockId = "";
  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoLock");
    expect(event?.args.inputs).to.deep.equal(nullifiers);
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.lockedOutputs).to.deep.equal(lockedOutputs);
    expect(event?.args.data).to.deep.equal(data);
    if (event?.args.lockId) {
      lockId = event.args.lockId;
    }
  }
  for (const input of nullifiers) {
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(false);
  }
  for (const output of lockedOutputs) {
    expect(await noto.isLocked(output)).to.equal(true);
    expect(await noto.isUnspent(output)).to.equal(false);
  }
  return lockId;
}

export async function doUnlock(
  txId: string,
  sender: Signer,
  noto: Noto,
  lockedInputs: string[],
  lockedOutputs: string[],
  outputs: string[],
  data: string,
  lockId: string,
  asNullifiers: boolean = false
) {
  const unlockParams = {
    lockedInputs,
    lockedOutputs,
    outputs,
    signature: "0x",
    data,
  };

  const tx = await noto.connect(sender).unlock(txId, lockId, unlockParams);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoUnlock");
    expect(event?.args.lockedInputs).to.deep.equal(lockedInputs);
    expect(event?.args.lockedOutputs).to.deep.equal(lockedOutputs);
    expect(event?.args.outputs).to.deep.equal(outputs);
    expect(event?.args.data).to.deep.equal(data);
  }
  for (const input of lockedInputs) {
    expect(await noto.isLocked(input)).to.equal(false);
    expect(await noto.isUnspent(input)).to.equal(false);
  }
  for (const output of lockedOutputs) {
    expect(await noto.isLocked(output)).to.equal(true);
    expect(await noto.isUnspent(output)).to.equal(false);
  }
  for (const output of outputs) {
    expect(await noto.isUnspent(output)).to.equal(asNullifiers ? false : true);
  }
}

export async function doPrepareUnlock(
  notary: Signer,
  noto: Noto,
  lockedInputs: string[],
  unlockHash: string,
  data: string,
  lockId: string
) {
  const txId = randomBytes32();
  const unlockTxId = randomBytes32();

  const tx = await noto
    .connect(notary)
    .prepareUnlock(
      txId,
      lockId,
      unlockTxId,
      lockedInputs,
      unlockHash,
      "0x",
      data
    );
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoUnlockPrepared");
    expect(event?.args.lockedInputs).to.deep.equal(lockedInputs);
    expect(event?.args.unlockHash).to.deep.equal(unlockHash);
    expect(event?.args.data).to.deep.equal(data);
  }
  for (const input of lockedInputs) {
    expect(await noto.isLocked(input)).to.equal(true);
  }
}

export async function doDelegateLock(
  txId: string,
  notary: Signer,
  noto: Noto,
  unlockHash: string,
  delegate: string,
  data: string,
  lockId: string
) {
  const tx = await noto
    .connect(notary)
    .delegateLock(txId, lockId, delegate, "0x", data);
  const results = await tx.wait();
  expect(results).to.exist;

  for (const log of results?.logs || []) {
    const event = noto.interface.parseLog(log);
    expect(event).to.exist;
    expect(event?.name).to.equal("NotoLockDelegated");
    expect(event?.args.lockId).to.deep.equal(lockId);
    expect(event?.args.delegate).to.deep.equal(delegate);
    expect(event?.args.data).to.deep.equal(data);
  }
}
