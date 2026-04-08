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

import i18next from 'i18next';
import { constants } from '../components/config';
import {
  IBlock,
  IEnrichedTransaction,
  IEvent,
  IFilter,
  IPaladinTransaction,
  ITransaction,
  ITransactionInput,
  ITransactionPagingReference,
  ITransactionReceipt,
} from '../interfaces';
import { translateFilters } from '../utils';
import { generatePostReq, returnResponse } from './common';
import { RpcEndpoint, RpcMethods } from './rpcMethods';

const getBlockNumberQuery = (blockNumber: number) => {
  return [
    {
      field: 'blockNumber',
      value: blockNumber,
    }
  ]
};

const getTransactionPagingQuery = (pageParam: ITransactionPagingReference) => {
  return [
    {
      lessThan: [
        {
          field: 'blockNumber',
          value: pageParam.blockNumber,
        }
      ]
    },
    {
      equal: [
        {
          field: 'blockNumber',
          value: pageParam.blockNumber,
        }
      ],
      lessThan: [
        {
          field: 'transactionIndex',
          value: pageParam.transactionIndex,
        }
      ]
    }
  ];
};

export const fetchIndexedTransactions = async (
  limit: number,
  fromBlockNumber?: number,
  pageParam?: ITransactionPagingReference
): Promise<IEnrichedTransaction[]> => {
  let requestPayload: any = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.bidx_QueryIndexedTransactions,
    params: [
      {
        limit,
        sort: ['blockNumber DESC', 'transactionIndex DESC'],
      },
    ],
  };

  if (fromBlockNumber !== undefined) {
    requestPayload.params[0].lessThanOrEqual = getBlockNumberQuery(fromBlockNumber);
  }
  if (pageParam !== undefined) {
    requestPayload.params[0].or = getTransactionPagingQuery(pageParam);
  }

  const transactions: ITransaction[] = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
    i18next.t('errorFetchingTransactions')
  );

  const receiptsResult = await fetchTransactionReceipts(transactions);
  const paladinTransactionsResult = await fetchPaladinTransactions(
    receiptsResult
  );
  const events = await fetchTransactionEvents(transactions);

  let enrichedTransactions: IEnrichedTransaction[] = [];

  for (const transaction of transactions) {
    enrichedTransactions.push({
      ...transaction,
      receipts: receiptsResult.filter(
        (receiptResult) => receiptResult.transactionHash === transaction.hash
      ),
      paladinTransactions: paladinTransactionsResult.filter(
        (paladinTransaction) =>
          receiptsResult
            ?.filter(
              (transactionReceipt) =>
                transactionReceipt.transactionHash === transaction.hash
            )
            .map((transactionReceipt) => transactionReceipt.id)
            .includes(paladinTransaction.id)
      ),
      events: events.filter(event => event.transactionHash === transaction.hash)
    });
  }

  return enrichedTransactions;
};

export const fetchSubmissions = async (
  type: 'all' | 'pending',
  filters: IFilter[],
  pageParam?: IPaladinTransaction
): Promise<IPaladinTransaction[]> => {
  let translatedFilters = translateFilters(filters);

  let allParams: any = [
    {
      ...translatedFilters,
      limit: constants.SUBMISSIONS_QUERY_LIMIT,
      sort: ['created DESC'],
    },
  ];

  if (pageParam !== undefined) {
    if (allParams[0].lessThan === undefined) {
      allParams[0].lessThan = [];
    }
    allParams[0].lessThan.push({
      field: 'created',
      value: pageParam.created,
    });
  }

  const pendingParams = [...allParams, true];
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method:
      type === 'all'
        ? RpcMethods.ptx_QueryTransactionsFull
        : RpcMethods.ptx_QueryPendingTransactions,
    params: type === 'all' ? allParams : pendingParams,
  };

  return <Promise<IPaladinTransaction[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingSubmissions')
    )
  );
};

export const fetchTransactionReceipt = async (
  transactionId: string
): Promise<ITransactionReceipt> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_getTransactionReceipt,
    params: [transactionId],
  };

  return <Promise<ITransactionReceipt>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingTransactionReceipt')
    )
  );
};

export const fetchTransactionReceipts = async (
  transactions: ITransaction[]
): Promise<ITransactionReceipt[]> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_QueryTransactionReceipts,
    params: [
      {
        limit: transactions.length * constants.RECEIPTS_PER_TRANSACTION_DEFAULT_LIMIT,
        in: [
          {
            field: 'transactionHash',
            values: transactions.map((transaction) =>
              transaction.hash.substring(2)
            ),
          },
        ],
      },
    ],
  };

  return <Promise<ITransactionReceipt[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingTransactionReceipts')
    )
  );
};

export const fetchPaladinTransactions = async (
  transactionReceipts: ITransactionReceipt[]
): Promise<IPaladinTransaction[]> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_QueryTransactionsFull,
    params: [
      {
        limit: transactionReceipts.length,
        in: [
          {
            field: 'id',
            values: transactionReceipts.map((transaction) => transaction.id),
          },
        ],
      },
    ],
  };

  return <Promise<IPaladinTransaction[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingPaladinTransactions')
    )
  );
};

export const fetchTransactionEvents = async (
  transactions: ITransaction[]
): Promise<IEvent[]> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.bidx_QueryIndexedEvents,
    params: [
      {
        limit: transactions.length * constants.EVENTS_PER_TRANSACTION_DEFAULT_LIMIT,
        in: [
          {
            field: 'transactionHash',
            values: transactions.map((transaction) =>
              transaction.hash.substring(2)
            ),
          },
        ],
      },
    ],
  };

  return <Promise<IEvent[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingEvents')
    )
  );
};

export const sendTransaction = async (
  transaction: ITransactionInput
): Promise<string> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_sendTransaction,
    params: [transaction],
  };
  console.log('Sending transaction');

  return <Promise<string>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorSendingTransaction')
    )
  );
};

export const fetchTransaction = async (
  hash: string
): Promise<IEnrichedTransaction | undefined> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.bidx_getTransactionByHash,
    params: [hash],
  };

  const transaction: ITransaction = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
    i18next.t('errorFetchingTransaction')
  );

  if (transaction === null) {
    return undefined;
  }

  const block = await fetchBlockByNumber(transaction.blockNumber);
  const receiptsResult = await fetchTransactionReceipts([transaction]);
  const paladinTransactionsResult = await fetchPaladinTransactions(
    receiptsResult
  );
  const events = await fetchTransactionEvents([transaction]);

  return {
    ...transaction,
    block,
    receipts: receiptsResult.filter(
      (receiptResult) => receiptResult.transactionHash === transaction.hash
    ),
    paladinTransactions: paladinTransactionsResult.filter(
      (paladinTransaction) =>
        receiptsResult
          ?.filter(
            (transactionReceipt) =>
              transactionReceipt.transactionHash === transaction.hash
          )
          .map((transactionReceipt) => transactionReceipt.id)
          .includes(paladinTransaction.id)
    ),
    events: events.filter(event => event.transactionHash === transaction.hash)
  };

};

export const fetchBlockByNumber = async (
  blockNumber: number
): Promise<IBlock> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.bidx_getBlockByNumber,
    params: [blockNumber],
  };

  return <Promise<IBlock>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingBlock')
    )
  );
};