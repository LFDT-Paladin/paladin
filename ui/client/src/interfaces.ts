// Copyright © 2026 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

export interface IBlock {
  number: number;
  hash: string;
  timestamp: string;
}

export interface ITransactionPagingReference {
  hash: string;
  blockNumber: number;
  transactionIndex: number;
}

export interface IPaladinTransactionPagingReference {
  id: string;
  created: string;
}

export interface ITransaction {
  hash: string;
  blockNumber: number;
  transactionIndex: number;
  from: string;
  to?: string;
  nonce: number;
  contractAddress?: string;
  result: string;
  block: IBlock;
}

export interface IEnrichedTransaction extends ITransaction {
  events: IEvent[];
  receipts: ITransactionReceipt[];
}

export interface IEvent {
  blockNumber: number;
  transactionIndex: number;
  logIndex: number;
  transactionHash: string;
  signature: string;
  block: IBlock;
}

export interface IRegistryEntry {
  registry: string;
  id: string;
  name: string;
  active: boolean;
  properties: {
    [key: string]: string;
  };
}

export interface IPaladinTransaction {
  id: string;
  created: string;
  type: string;
  domain?: string;
  function: string;
  to?: string;
  from: string;
  abiReference: string;
  data: {
    [key: string]: string;
  };
  receipt?: ITransactionReceipt;
}

export interface ITransactionReceipt {
  blockNumber: number;
  domain?: string;
  id: string;
  success: boolean;
  transactionHash: string;
  failureMessage?: string;
}

export interface IStateReceipt {
  none?: boolean;
  [key: string]: any;
}

export interface IPrivateEVMTransaction {
  from?: string;
  to?: string;
  nonce?: string;
  gas?: string;
  data?: string;
}

export interface IPrivateEVMReceipt {
  from?: string;
  to?: string;
  gasUsed?: string;
  contractAddress?: string;
  logs?: IPrivateEVMLog[];
}

export interface IPrivateEVMLog {
  address?: string;
  topics?: string[];
  data?: string;
}

export interface IABIParameter {
  type: string;
  name?: string;
  indexed?: boolean;
  components?: IABIParameter[] | null;
}

export interface IABIEntry {
  type: string;
  name: string;
  inputs?: IABIParameter[] | null;
  outputs?: IABIParameter[] | null;
}

export interface IABIDecodedEntry {
  data: any;
  definition: IABIEntry;
  signature: string;
  summary?: string; // errors only
}

export type ABIUploadResponse = string;

export interface ITransportPeer {
  name: string;
  stats: {
    sentMsgs: number;
    receivedMsgs: number;
    sentBytes: number;
    receivedBytes: number;
    lastSend: string;
    lastReceive: string | null;
    reliableHighestSent: number;
    reliableAckBase: number;
  };
  activated: string;
  outboundTransport: string;
  outbound: {
    endpoint: string;
  };
}

export interface IVerifier {
  verifier: string;
  type: string;
  algorithm: string;
}

export interface IKeyEntry {
  isKey: boolean;
  hasChildren: boolean;
  path: string;
  index: number;
  type: string;
  verifiers: IVerifier[] | null;
  wallet: string;
  keyHandle: string;
}

export interface IKeyMappingAndVerifier {
  identifier: string;
  keyHandle: string;
  path: {
    index: number;
    name: string;
  }[];
  verifier: IVerifier;
  wallet: string;
}

export interface IFilterField {
  label: string;
  name: string;
  type: 'string' | 'number' | 'boolean';
  isUUID?: boolean;
  isHexValue?: boolean;
  emun?: string[];
}

export interface IFilter {
  field: IFilterField;
  operator: string;
  value: string;
  caseSensitive?: boolean;
}

export enum TransactionType {
  PUBLIC = 'public',
  PRIVATE = 'private',
}

export interface ITransactionInput {
  type: TransactionType;
  domain?: string;
  function?: string;
  from: string;
  to?: string;
  data: {
    [key: string]: any;
  };
  abiReference?: string;
  abi?: any;
  bytecode?: string;
}

export interface INotoContractConfig {
  name: string
  symbol: string
  isNotary: boolean
}

export interface IZetoContractConfig {
  tokenName: string
}

export type ContractConfig = INotoContractConfig | IZetoContractConfig;

export interface IDomainContract {
  domainName: string
  domainAddress: string
  address: string
  config: {
    contractConfig: ContractConfig
  }
  created: string
}

export interface IPrivacyGroup {
  id: string
  domain: string
  created: string
  name: string,
  members: string[]
  properties: any
  configuration: {
    endorsementType: string
    evmVersion: string
    externalCallsEnabled: boolean
  },
  genesisSalt: string
  genesisSchema: string
  genesisTransaction: string
  contractAddress: string
}

export interface ISchemaComponent {
  name: string
  type: string
  indexed: boolean
}

export interface ISchema {
  id: string
  domain: string
  type: string
  signature: string
  definition: {
    name: string
    type: string
    internalType: string
    components: ISchemaComponent[]
  }
  labels: string[]
}

export interface IState {
  id: string
  created: string
  domain: string
  schema: string
  contractAddress: string | null
  data: any
}

export interface IMessage {
  sequence: string
  id: string
  created: string
  node: string
  messageType: string
  metadata: any
  ack?: {
    time: string
  }
}