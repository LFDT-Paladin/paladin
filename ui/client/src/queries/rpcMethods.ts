// Copyright contributors to Paladin, an LFDT project
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

import { getBasePath } from '../utils';

export const RpcEndpoint = getBasePath();

const methods = [
  'bidx_getBlockByNumber',
  'bidx_getTransactionByHash',
  'bidx_queryIndexedEvents',
  'bidx_queryIndexedTransactions',
  'bidx_queryIndexedTransactionsWithReceipt',
  'domain_getDomain',
  'domain_getSmartContractByAddress',
  'domain_listDomains',
  'domain_querySmartContracts',
  'keymgr_queryKeys',
  'keymgr_reverseKeyLookup',
  'pgroup_getGroupByAddress',
  'pgroup_getGroupById',
  'pgroup_queryMessages',
  'pgroup_queryGroups',
  'pgroup_sendMessage',
  'pgroup_createGroup',
  'pgroup_queryMessageListeners',
  'pgroup_startMessageListener',
  'pgroup_stopMessageListener',
  'pgroup_getMessageListener',
  'pgroup_createMessageListener',
  'pgroup_deleteMessageListener',
  'pstate_listSchemas',
  'pstate_queryStates',
  'pstate_transferPrivateState',
  'ptx_call',
  'ptx_createBlockchainEventListener',
  'ptx_createReceiptListener',
  'ptx_decodeCall',
  'ptx_decodeEvent',
  'ptx_deleteBlockchainEventListener',
  'ptx_deleteReceiptListener',
  'ptx_getBlockchainEventListener',
  'ptx_getDomainReceipt',
  'ptx_getReceiptListener',
  'ptx_getStateReceipt',
  'ptx_getTransaction',
  'ptx_getTransactionFull',
  'ptx_getTransactionReceipt',
  'ptx_getTransactionReceiptFull',
  'ptx_queryBlockchainEventListeners',
  'ptx_queryReceiptListeners',
  'ptx_queryPendingTransactions',
  'ptx_queryTransactionReceipts',
  'ptx_queryTransactions',
  'ptx_queryTransactionsFull',
  'ptx_resolveVerifier',
  'ptx_sendTransaction',
  'ptx_startBlockchainEventListener',
  'ptx_startReceiptListener',
  'ptx_stopBlockchainEventListener',
  'ptx_stopReceiptListener',
  'ptx_storeABI',
  'reg_queryEntriesWithProps',
  'reg_registries',
  'transport_localTransportDetails',
  'transport_nodeName',
  'transport_peers',
  'transport_queryReliableMessages',
  'transport_queryPeers',
] as const;

type MethodType = typeof methods[number];

export const RpcMethods = Object.fromEntries(
  methods.map((m) => [m, m])
) as Record<MethodType, MethodType>;
