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

import { getBasePath } from '../utils';

export const RpcEndpoint = getBasePath();

export const RpcMethods = {
  bidx_getBlockByNumber: 'bidx_getBlockByNumber',
  bidx_getTransactionByHash: 'bidx_getTransactionByHash',
  bidx_QueryIndexedEvents: 'bidx_queryIndexedEvents',
  bidx_QueryIndexedTransactions: 'bidx_queryIndexedTransactions',
  bidx_QueryIndexedTransactionsWithReceipt: 'bidx_queryIndexedTransactionsWithReceipt',
  domain_getDomain: 'domain_getDomain',
  domain_getSmartContractByAddress: 'domain_getSmartContractByAddress',
  domain_listDomains: 'domain_listDomains',
  domain_querySmartContracts: 'domain_querySmartContracts',
  keymgr_queryKeys: 'keymgr_queryKeys',
  keymgr_reverseKeyLookup: 'keymgr_reverseKeyLookup',
  pgroup_getGroupByAddress: 'pgroup_getGroupByAddress',
  pgroup_getGroupById: 'pgroup_getGroupById',
  pgroup_queryMessages: 'pgroup_queryMessages',
  pgroup_queryGroups: 'pgroup_queryGroups',
  pgroup_sendMessage: 'pgroup_sendMessage',
  pgroup_createGroup: 'pgroup_createGroup',
  pgroup_queryMessageListeners: 'pgroup_queryMessageListeners',
  pgroup_startMessageListener: 'pgroup_startMessageListener',
  pgroup_stopMessageListener: 'pgroup_stopMessageListener',
  pgroup_getMessageListener: 'pgroup_getMessageListener',
  pstate_listSchemas: 'pstate_listSchemas',
  pstate_queryStates: 'pstate_queryStates',
  pstate_transferPrivateState: 'pstate_transferPrivateState',
  ptx_call: 'ptx_call',
  ptx_decodeCall: 'ptx_decodeCall',
  ptx_decodeEvent: 'ptx_decodeEvent',
  ptx_getDomainReceipt: 'ptx_getDomainReceipt',
  ptx_getStateReceipt: 'ptx_getStateReceipt',
  ptx_getTransaction: 'ptx_getTransaction',
  ptx_getTransactionFull: 'ptx_getTransactionFull',
  ptx_getTransactionReceipt: 'ptx_getTransactionReceipt',
  ptx_getTransactionReceiptFull: 'ptx_getTransactionReceiptFull',
  ptx_QueryPendingTransactions: 'ptx_queryPendingTransactions',
  ptx_QueryTransactionReceipts: 'ptx_queryTransactionReceipts',
  ptx_QueryTransactions: 'ptx_queryTransactions',
  ptx_QueryTransactionsFull: 'ptx_queryTransactionsFull',
  ptx_resolveVerifier: 'ptx_resolveVerifier',
  ptx_sendTransaction: 'ptx_sendTransaction',
  ptx_storeABI: 'ptx_storeABI',
  reg_QueryEntriesWithProps: 'reg_queryEntriesWithProps',
  reg_Registries: 'reg_registries',
  transport_localTransportDetails: 'transport_localTransportDetails',
  transport_nodeName: 'transport_nodeName',
  transport_peers: 'transport_peers',
  transport_queryReliableMessages: 'transport_queryReliableMessages',
  transport_queryPeers: 'transport_queryPeers'
};
