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

import i18next from 'i18next';
import { constants } from '../components/config';
import {
  IBlock,
  ICreateEventListenerParams,
  ICreateReceiptListenerParams,
  IEnrichedTransaction,
  IEvent,
  IEventListener,
  IFetchIndexedTransactionsParams,
  IFetchSubmissionsParams,
  IPaladinTransaction,
  IPaladinTransactionPagingReference,
  IPagedQueryParams,
  IReceiptListener,
  ITransaction,
  ITransactionInput,
  IPagedResult,
  ISortPagingReference,
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
  params: IFetchIndexedTransactionsParams
): Promise<IPagedResult<IEnrichedTransaction>> => {
  const { limit, withReceipt, filters, pageParam } = params;
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

export const buildPaladinTransactionPagingReference = (
  transaction: IPaladinTransaction
): IPaladinTransactionPagingReference => ({
  id: transaction.id,
  created: transaction.created,
});

export const fetchSubmissions = async (
  params: IFetchSubmissionsParams
): Promise<IPagedResult<IPaladinTransaction>> => {
  const { type, limit, filters, sortAscending, pageParam } = params;
  let translatedFilters = translateFilters(filters);
  const sortDirection = sortAscending ? 'ASC' : 'DESC';

  let queryParams: any = {
    ...translatedFilters,
    limit: limit + 1,
    sort: [
      `created ${sortDirection}`,
      `id ${sortDirection}`,
    ],
  };

  if (pageParam !== undefined) {
    const comparison = sortAscending ? 'greaterThan' : 'lessThan';
    queryParams.or = [
      {
        [comparison]: [{
          field: 'created',
          value: pageParam.created,
        }],
      },
      {
        equal: [{
          field: 'created',
          value: pageParam.created,
        }],
        [comparison]: [{
          field: 'id',
          value: pageParam.id,
        }],
      },
    ];
  }

  let rpcParams: any = [queryParams];

  if (['failed', 'successful'].includes(type)) {
    if (rpcParams[0].equal === undefined) {
      rpcParams[0].equal = [];
    }
    rpcParams[0].equal.push(
      {
        field: 'success',
        value: type === 'successful'
      }
    );
  } else {
    rpcParams = [...rpcParams, true];
  }

  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method:
      type === 'pending'
        ? RpcMethods.ptx_QueryPendingTransactions
        : RpcMethods.ptx_QueryTransactionsFull,
    params: rpcParams
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

export const getEventListenerSortValue = (
  listener: IEventListener,
  sortBy: string,
): any => sortBy === 'created' ? listener.created : listener.name;

export const buildEventListenerPagingReference = (
  listener: IEventListener,
  sortBy: string,
): ISortPagingReference => ({
  sortValue: getEventListenerSortValue(listener, sortBy),
  tiebreaker: listener.name,
});

export const listEventListeners = async (
  params: IPagedQueryParams
): Promise<IPagedResult<IEventListener>> => {
  const { limit, filters, sortBy, sortAscending, pageRef } = params;
  let translatedFilters = translateFilters(filters);
  const sortDirection = sortAscending ? 'ASC' : 'DESC';

  let queryParams: any = {
    ...translatedFilters,
    limit: limit + 1,
    sort: [
      `${sortBy} ${sortDirection}`,
      `name ${sortDirection}`,
    ],
  };

  if (pageRef !== undefined) {
    const comparison = sortAscending ? 'greaterThan' : 'lessThan';
    queryParams.or = [
      {
        [comparison]: [{
          field: sortBy,
          value: pageRef.sortValue,
        }],
      },
      {
        equal: [{
          field: sortBy,
          value: pageRef.sortValue,
        }],
        [comparison]: [{
          field: 'name',
          value: pageRef.tiebreaker,
        }],
      },
    ];
  }

  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_queryBlockchainEventListeners,
    params: [queryParams]
  };
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
    i18next.t('errorFetchingEventListeners')
  );
  return toPagedResult(results, limit);
};

export const startEventListener = async (
  listenerName: string
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_startBlockchainEventListener,
    params: [listenerName],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorStartingEventListener')
    )
  );
};

export const stopEventListener = async (
  listenerName: string
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_stopBlockchainEventListener,
    params: [listenerName],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorStoppingEventListener')
    )
  );
};

export const createEventListener = async (
  params: ICreateEventListenerParams
): Promise<boolean> => {
  const { name, started, sources, options } = params;
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_createBlockchainEventListener,
    params: [{
      name,
      started,
      sources,
      options
    }],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorCreatingEventListener')
    )
  );
};

export const deleteEventListener = async (
  listenerName: string
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_deleteBlockchainEventListener,
    params: [listenerName],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorDeletingEventListener')
    )
  );
};

export const getEventListener = async (
  listenerName: string
): Promise<IEventListener> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_getBlockchainEventListener,
    params: [listenerName],
  };
  return <Promise<IEventListener>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingEventListener')
    )
  );
};

export const getReceiptListenerSortValue = (
  listener: IReceiptListener,
  sortBy: string,
): any => sortBy === 'created' ? listener.created : listener.name;

export const buildReceiptListenerPagingReference = (
  listener: IReceiptListener,
  sortBy: string,
): ISortPagingReference => ({
  sortValue: getReceiptListenerSortValue(listener, sortBy),
  tiebreaker: listener.name,
});

export const listReceiptListeners = async (
  params: IPagedQueryParams
): Promise<IPagedResult<IReceiptListener>> => {
  const { limit, filters, sortBy, sortAscending, pageRef } = params;
  let translatedFilters = translateFilters(filters);
  const sortDirection = sortAscending ? 'ASC' : 'DESC';

  let queryParams: any = {
    ...translatedFilters,
    limit: limit + 1,
    sort: [
      `${sortBy} ${sortDirection}`,
      `name ${sortDirection}`,
    ],
  };

  if (pageRef !== undefined) {
    const comparison = sortAscending ? 'greaterThan' : 'lessThan';
    queryParams.or = [
      {
        [comparison]: [{
          field: sortBy,
          value: pageRef.sortValue,
        }],
      },
      {
        equal: [{
          field: sortBy,
          value: pageRef.sortValue,
        }],
        [comparison]: [{
          field: 'name',
          value: pageRef.tiebreaker,
        }],
      },
    ];
  }

  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_queryReceiptListeners,
    params: [queryParams]
  };
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
    i18next.t('errorFetchingReceiptListeners')
  );
  return toPagedResult(results, limit);
};

export const startReceiptListener = async (
  listenerName: string
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_startReceiptListener,
    params: [listenerName],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorStartingReceiptListener')
    )
  );
};

export const stopReceiptListener = async (
  listenerName: string
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_stopReceiptListener,
    params: [listenerName],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorStoppingReceiptListener')
    )
  );
};

export const createReceiptListener = async (
  params: ICreateReceiptListenerParams
): Promise<boolean> => {
  const { name, started, filters, options } = params;
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_createReceiptListener,
    params: [{
      name,
      started,
      filters,
      options
    }],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorCreatingReceiptListener')
    )
  );
};

export const deleteReceiptListener = async (
  listenerName: string
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_deleteReceiptListener,
    params: [listenerName],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorDeletingReceiptListener')
    )
  );
};

export const getReceiptListener = async (
  listenerName: string
): Promise<IReceiptListener> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_getReceiptListener,
    params: [listenerName],
  };
  return <Promise<IReceiptListener>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingReceiptListener')
    )
  );
};
