import { TransactionType } from "../interfaces";
import PaladinClient from "../paladin";
import { TransactionFuture } from "../transaction";
import { PaladinVerifier } from "../verifier";
import * as zetoPrivateJSON from "./abis/IZetoFungible.json";
import * as zetoPrivateV1JSON from "./abis/IZetoFungible_V1.json";
import * as zetoPublicJSON from "./abis/Zeto_Anon.json";

// Algorithm/verifier types specific to Zeto
export const algorithmZetoSnarkBJJ = (domainName: string) =>
  `domain:${domainName}:snark:babyjubjub`;
export const IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X =
  "iden3_pubkey_babyjubjub_compressed_0x";

const zetoAbi = zetoPrivateJSON.abi;
// The V1 transaction API: createLock / spendLock / cancelLock in place of V0's lock / transferLocked.
const zetoAbiV1 = zetoPrivateV1JSON.abi;
const zetoPublicAbi = zetoPublicJSON.abi;

export const zetoConstructorABI = {
  type: "constructor",
  inputs: [{ name: "tokenName", type: "string" }],
};

// Opts a pool into the V1 transaction axis. The generation decides the on-chain calldata shape — zeto-contracts
// ~v0.5.x takes a single packed `bytes proof` where ~v0.2.x took a discrete proof tuple — so a pool deployed without
// these against v0.5.x implementations reverts on the first proving call (deposit) with no decodable revert data.
export const zetoConstructorABI_V1 = {
  type: "constructor",
  inputs: [
    { name: "tokenName", type: "string" },
    { name: "domainConfigSchema", type: "string" },
    { name: "zetoVariant", type: "uint256" },
    { name: "factoryVersion", type: "uint256" },
  ],
};

export interface ZetoCreateLockParams {
  from: string;
  recipients: ZetoTransfer[];
  unlockData: string;
  data: string;
}

export interface ZetoSpendLockParams {
  lockId: string;
  from: string;
  data: string;
}

// V1 delegateLock on the pool: delegateLock(bytes32 lockId, bytes delegateArgs, address newSpender, bytes data).
export interface ZetoDelegateLockV1Params {
  lockId: string;
  delegateArgs: string;
  newSpender: string;
  data: string;
}

export interface ZetoConstructorParams {
  tokenName: string;
  // Supply all three together to deploy on the V1 axis; omit them all for the legacy V0 axis.
  domainConfigSchema?: string;
  zetoVariant?: number;
  factoryVersion?: number;
}

export interface ZetoMintParams {
  mints: ZetoTransfer[];
}

export interface ZetoTransferParams {
  transfers: ZetoTransfer[];
}

export interface ZetoLockParams {
  amount: number;
  delegate: string;
}

export interface ZetoTransferLockedParams {
  lockedInputs: string[];
  delegate: string;
  transfers: ZetoTransfer[];
}

export interface ZetoDelegateLockParams {
  utxos: string[];
  delegate: string;
}

export interface ZetoSetERC20Params {
  erc20: string;
}

export interface ZetoTransfer {
  to: PaladinVerifier;
  amount: string | number;
  data: string;
}

export interface ZetoDepositParams {
  amount: string | number;
}

export interface ZetoWithdrawParams {
  amount: string | number;
}

export interface ZetoBalanceOfParams {
  account: string;
}

export interface ZetoBalanceOfResult {
  totalBalance: string;
  totalStates: string;
  overflow: boolean;
}

// Represents an in-flight Zeto deployment
export class ZetoFuture extends TransactionFuture {
  async waitForDeploy(waitMs?: number) {
    const receipt = await this.waitForReceipt(waitMs);
    return receipt?.contractAddress
      ? new ZetoInstance(this.paladin, receipt.contractAddress)
      : undefined;
  }
}

export class ZetoFactory {
  constructor(private paladin: PaladinClient, public readonly domain: string) {}

  using(paladin: PaladinClient) {
    return new ZetoFactory(paladin, this.domain);
  }

  newZeto(from: PaladinVerifier, data: ZetoConstructorParams) {
    // The V1 constructor carries three extra params, so the ABI has to match what is actually being sent.
    const abi =
      data.domainConfigSchema !== undefined ||
      data.zetoVariant !== undefined ||
      data.factoryVersion !== undefined
        ? zetoConstructorABI_V1
        : zetoConstructorABI;
    return new ZetoFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        domain: this.domain,
        abi: [abi],
        function: "",
        from: from.lookup,
        data,
      })
    );
  }
}

export class ZetoInstance {
  private erc20?: string;

  constructor(
    private paladin: PaladinClient,
    public readonly address: string
  ) {}

  using(paladin: PaladinClient) {
    const zeto = new ZetoInstance(paladin, this.address);
    zeto.erc20 = this.erc20;
    return zeto;
  }

  mint(from: PaladinVerifier, data: ZetoMintParams) {
    const params = {
      mints: data.mints.map((t) => ({ ...t, to: t.to.lookup })),
    };
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "mint",
        to: this.address,
        from: from.lookup,
        data: params,
      })
    );
  }

  transfer(from: PaladinVerifier, data: ZetoTransferParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "transfer",
        to: this.address,
        from: from.lookup,
        data: {
          transfers: data.transfers.map((t) => ({ ...t, to: t.to.lookup })),
        },
      })
    );
  }

  transferLocked(from: PaladinVerifier, data: ZetoTransferLockedParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "transferLocked",
        to: this.address,
        from: from.lookup,
        data: {
          lockedInputs: data.lockedInputs,
          delegate: data.delegate,
          transfers: data.transfers.map((t) => ({ ...t, to: t.to.lookup })),
        },
      })
    );
  }

  prepareTransferLocked(from: PaladinVerifier, data: ZetoTransferLockedParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.prepareTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "transferLocked",
        to: this.address,
        from: from.lookup,
        data: {
          lockedInputs: data.lockedInputs,
          delegate: data.delegate,
          transfers: data.transfers.map((t) => ({ ...t, to: t.to.lookup })),
        },
      })
    );
  }

  lock(from: PaladinVerifier, data: ZetoLockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "lock",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  // --- V1 lock lifecycle (IZetoFungible_V1) ----------------------------------------------------------------
  // V1 replaces V0's lock/transferLocked pair: the spend recipients are pinned when the lock is created, so the
  // delegate's spendLock can be encoded and authorised ahead of time.

  createLock(from: PaladinVerifier, data: ZetoCreateLockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbiV1,
        function: "createLock",
        to: this.address,
        from: from.lookup,
        data: {
          from: data.from,
          recipients: data.recipients.map((t) => ({ ...t, to: t.to.lookup })),
          unlockData: data.unlockData,
          data: data.data,
        },
      })
    );
  }

  prepareSpendLock(from: PaladinVerifier, data: ZetoSpendLockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.prepareTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbiV1,
        function: "spendLock",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  // Hands the lock to a new spender. Only the current spender — the submitter of the public createLock — may call it.
  delegateLockV1(from: PaladinVerifier, data: ZetoDelegateLockV1Params) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PUBLIC,
        abi: zetoPublicAbi,
        function: "delegateLock",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  delegateLock(from: PaladinVerifier, data: ZetoDelegateLockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PUBLIC,
        abi: zetoPublicAbi,
        function: "delegateLock",
        to: this.address,
        from: from.lookup,
        data: {
          data: "0x",
          utxos: data.utxos,
          delegate: data.delegate,
        },
      })
    );
  }

  setERC20(from: PaladinVerifier, data: ZetoSetERC20Params) {
    this.erc20 = data.erc20;
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PUBLIC,
        abi: zetoAbi,
        function: "setERC20",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  deposit(from: PaladinVerifier, data: ZetoDepositParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "deposit",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  withdraw(from: PaladinVerifier, data: ZetoWithdrawParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "withdraw",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  async balanceOf(
    from: PaladinVerifier,
    data: ZetoBalanceOfParams
  ): Promise<ZetoBalanceOfResult> {
    return await this.paladin.call({
      type: TransactionType.PRIVATE,
      domain: "zeto",
      abi: zetoPrivateJSON.abi,
      function: "balanceOf",
      to: this.address,
      from: from.lookup,
      data,
    });
  }
}
