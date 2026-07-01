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
  IPaladinTransactionPagingReference,
  ITransaction,
  ITransactionInput,
  IPagedResult,
  ITransactionPagingReference,
  ITransactionReceipt,
} from '../interfaces';
import { toPagedResult, translateFilters } from '../utils';
import { generatePostReq, returnResponse } from './common';
import { RpcEndpoint, RpcMethods } from './rpcMethods';

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
  withReceipt: boolean,
  filters: IFilter[],
  pageParam?: ITransactionPagingReference
): Promise<IPagedResult<IEnrichedTransaction>> => {
  let translatedFilters = translateFilters(filters);

  let requestPayload: any = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: withReceipt ? RpcMethods.bidx_QueryIndexedTransactionsWithReceipt : RpcMethods.bidx_QueryIndexedTransactions,
    params: [
      {
        ...translatedFilters,
        limit: limit + 1,
        sort: ['blockNumber DESC', 'transactionIndex DESC'],
      }
    ]
  };

  if (pageParam !== undefined) {
    requestPayload.params[0].or = getTransactionPagingQuery(pageParam);
  }

  const transactions: ITransaction[] = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
    i18next.t('errorFetchingTransactions')
  );

  const { items: pageTransactions, hasMore } = toPagedResult(transactions, limit);

  const receiptsResult = await fetchTransactionReceipts(pageTransactions);
  const events = await fetchTransactionEvents(pageTransactions);

  let enrichedTransactions: IEnrichedTransaction[] = [];

  for (const transaction of pageTransactions) {
    enrichedTransactions.push({
      ...transaction,
      receipts: receiptsResult.filter(
        (receiptResult) => receiptResult.transactionHash === transaction.hash
      ),
      events: events.filter(event => event.transactionHash === transaction.hash)
    });
  }

  return { items: enrichedTransactions, hasMore };
};

export const fetchSubmissions = async (
  type: 'pending' | 'failed' | 'successful',
  limit: number,
  filters: IFilter[],
  sortAscending?: boolean,
  pageParam?: IPaladinTransactionPagingReference
): Promise<IPagedResult<IPaladinTransaction>> => {
  let translatedFilters = translateFilters(filters);

  let params: any = [
    {
      ...translatedFilters,
      limit: limit + 1,
      sort: [`created ${sortAscending ? 'ASC' : 'DESC'}`],
      greaterThan: pageParam !== undefined && sortAscending ? [
        {
          field: 'created',
          value: pageParam.created
        }
      ] : undefined,
      lessThan: pageParam !== undefined && !sortAscending ? [
        {
          field: 'created',
          value: pageParam.created
        }
      ] : undefined
    },
  ];

  if (['failed', 'successful'].includes(type)) {
    if (params[0].equal === undefined) {
      params[0].equal = [];
    }
    params[0].equal.push(
      {
        field: 'success',
        value: type === 'successful'
      }
    );
  } else {
    params = [...params, true];
  }

  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method:
      type === 'pending'
        ? RpcMethods.ptx_QueryPendingTransactions
        : RpcMethods.ptx_QueryTransactionsFull,
    params
  };

  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
    i18next.t('errorFetchingSubmissions')
  );
  return toPagedResult(results, limit);
};

export const fetchTransactionReceipt = async (
  id: string
): Promise<ITransactionReceipt> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_getTransactionReceipt,
    params: [id],
  };

  return <Promise<ITransactionReceipt>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingTransactionReceipt')
    )
  );
};

export const fetchTransactionReceiptFull = async (
  transactionId: string
): Promise<ITransactionReceipt> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_getTransactionReceiptFull,
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
        limit: (transactions.length + 1) * constants.RECEIPTS_PER_TRANSACTION_DEFAULT_LIMIT,
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
        limit: transactionReceipts.length + 1,
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
        limit: (transactions.length + 1) * constants.EVENTS_PER_TRANSACTION_DEFAULT_LIMIT,
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
  return <Promise<string>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorSendingTransaction')
    )
  );
};

export const fetchEnrichedTransaction = async (
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
  const events = await fetchTransactionEvents([transaction]);

  return {
    ...transaction,
    block,
    receipts: receiptsResult.filter(
      (receiptResult) => receiptResult.transactionHash === transaction.hash
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

export const fetchPaladinTransaction = async (
  id: string
): Promise<IPaladinTransaction> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_getTransaction,
    params: [id]
  };
  return <Promise<IPaladinTransaction>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingPaladinTransaction')
    )
  );
};

export const fetchPaladinTransactionFull = async (
  id: string
): Promise<IPaladinTransaction | null> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_getTransactionFull,
    params: [id]
  };
  return <Promise<IPaladinTransaction>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingPaladinTransaction')
    )
  );
};
