import { BigNumberish, ethers } from "ethers";
import { IStateBase } from "./states";

export interface IBlock {
  number: number;
  hash: string;
  timestamp: string;
}

export enum TransactionType {
  PUBLIC = "public",
  PRIVATE = "private",
}

export interface PublicTxOptions {
  gas?: BigNumberish;
  value?: BigNumberish;
  maxPriorityFeePerGas?: BigNumberish;
  maxFeePerGas?: BigNumberish;
}

export interface PublicCallOptions {
  block?: BigNumberish | string;
}

export interface ITransactionBase {
  idempotencyKey?: string;
  type: TransactionType;
  domain?: string;
  function?: string;
  from: string;
  to?: string;
  data: {
    [key: string]: any;
  };
}

export type SubmitMode = "auto" | "external" | "call" | "prepare";

export interface ITransaction extends ITransactionBase {
  id: string;
  created: string;
  abiReference: string;
  submitMode?: SubmitMode;
}

export interface IPreparedTransaction {
  id: string;
  domain: string;
  to: string;
  transaction: ITransactionBase & {
    abiReference: string;
  };
  states: {
    spent?: IStateBase[];
    read?: IStateBase[];
    confirmed?: IStateBase[];
    info?: IStateBase[];
  };
  metadata: any;
}

export interface ITransactionInput extends ITransactionBase {
  abiReference?: string;
  abi?: ethers.InterfaceAbi;
  bytecode?: string;
  dependsOn?: string[];
}

export interface ITransactionCall extends ITransactionInput, PublicCallOptions {
  dataFormat?: string;
}

export interface ITransactionReceipt {
  blockNumber: number;
  id: string;
  sequence: number;
  success: boolean;
  transactionHash: string;
  source: string;
  domain?: string;
  contractAddress?: string;
  states?: ITransactionStates;
  domainReceipt?: IPenteDomainReceipt | INotoDomainReceipt;
  domainReceiptError?: string
  failureMessage?: string;
}

export interface IPenteDomainReceipt {
  receipt: {
    from?: string;
    to?: string;
    contractAddress?: string;
    logs?: IPenteLog[];
  };
}

export interface IPenteLog {
  address: string;
  topics: string[];
  data: string;
}

// V1 lock info state data
export interface INotoLockInfoV1 {
  salt: string;
  lockId: string;
  owner: string;
  spender: string;
  replaces: string;
  spendTxId: string;
  spendOutputs: string[];
  spendData: string;
  cancelOutputs: string[];
  cancelData: string;
}

// V1 spendLock params in receipt
export interface INotoSpendLockParams {
  lockId: string;
  spendArgs: string;
  data: string;
}

// V1 cancelLock params in receipt
export interface INotoCancelLockParams {
  lockId: string;
  cancelArgs: string;
  data: string;
}

// Legacy unlock params in receipt
export interface INotoLegacyUnlockParams {
  txId: string;
  lockedInputs: string[];
  lockedOutputs: string[];
  outputs: string[];
  signature: string;
  data: string;
}

export interface INotoDomainReceipt {
  states: {
    inputs?: IReceiptState<INotoCoin>[];
    outputs?: IReceiptState<INotoCoin>[];
    readInputs?: IReceiptState<INotoCoin>[];
    preparedOutputs?: IReceiptState<INotoCoin>[];

    lockedInputs?: IReceiptState<INotoLockedCoin>[];
    lockedOutputs?: IReceiptState<INotoLockedCoin>[];
    readLockedInputs?: IReceiptState<INotoLockedCoin>[];
    preparedLockedOutputs?: IReceiptState<INotoLockedCoin>[];
    updatedLockInfo?: IReceiptState<INotoLockInfoV1>[];
  };
  transfers?: {
    from?: string;
    to?: string;
    amount: string;
  }[];
  lockInfo?: INotoReceiptLockInfo;
  data?: string;
}

export interface INotoReceiptLockInfo {
  lockId: string;
  delegate?: string;
  spendTxId?: string;
  unlockFunction?: string;
  unlockParams?: INotoSpendLockParams | INotoLegacyUnlockParams;
  unlockCall?: string;
  cancelFunction?: string;
  cancelParams?: INotoCancelLockParams;
  cancelCall?: string;
}

export interface IReceiptState<T> {
  id: string;
  schema: string;
  data: T;
}

export interface INotoCoin {
  salt: string;
  owner: string;
  amount: string;
}

export interface INotoLockedCoin {
  lockId: string;
  salt: string;
  owner: string;
  amount: string;
}

export interface ITransactionStates {
  none?: boolean;
  spent?: IStateBase[];
  read?: IStateBase[];
  confirmed?: IStateBase[];
  info?: IStateBase[];
  unavailable?: {
    spent?: string[];
    read?: string[];
    confirmed?: string[];
    info?: string[];
  };
}

export interface IABIDecodedData {
  signature: string;
  definition: ethers.JsonFragment;
  data: any;
  summary: string; // errors only
}

export interface IStoredABI {
  hash: string;
  abi: ethers.InterfaceAbi;
}

export interface ITransactionReceiptListener {
  name: string;
  filters?: {
    sequenceAbove?: number;
    type?: TransactionType;
    domain?: string;
  };
  options?: {
    domainReceipts?: boolean;
    incompleteStateReceiptBehavior?: "block_contract" | "process" | "complete_only";
  };
}
