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
import { generatePostReq, returnResponse } from './common';
import { RpcEndpoint, RpcMethods } from './rpcMethods';
import { IFilter, IPagedResult, IPrivacyGroup, IPrivacyGroupListener, IPrivacyGroupMessage, IPrivacyGroupMessageListenerFilters, IPrivacyGroupMessageListenerOptions, IPrivacyGroupPagingReference, ISortPagingReference } from '../interfaces';
import { deepMerge, toPagedResult, translateFilters } from '../utils';

export const getPrivacyGroupSortValue = (privacyGroup: IPrivacyGroup, sortBy: string): any => {
  if (sortBy === 'created') {
    return privacyGroup.created;
  }
  return privacyGroup.name;
};

export const buildPrivacyGroupPagingReference = (
  privacyGroup: IPrivacyGroup,
  sortBy: string,
): IPrivacyGroupPagingReference => ({
  sortValue: getPrivacyGroupSortValue(privacyGroup, sortBy),
  id: privacyGroup.id,
});

export const listPrivacyGroups = async (
  limit: number,
  filters: IFilter[],
  sortBy: string,
  sortAscending: boolean,
  pageRef?: IPrivacyGroupPagingReference
): Promise<IPagedResult<IPrivacyGroup>> => {
  let translatedFilters = translateFilters(filters);
  const sortDirection = sortAscending ? 'ASC' : 'DESC';

  let queryParams: any = {
    ...translatedFilters,
    limit: limit + 1,
    sort: [
      `${sortBy} ${sortDirection}`,
      `id ${sortDirection}`,
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
          field: 'id',
          value: pageRef.id,
        }],
      },
    ];
  }

  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_queryGroups,
    params: [queryParams]
  };
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
    i18next.t('errorFetchingPrivacyGroups')
  );
  return toPagedResult(results, limit);
};

export const getPrivacyGroupById = async (id: string): Promise<IPrivacyGroup> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_getGroupById,
    // Note: we are temporarily sending "pente" as the domain argument here as there is an ongoing
    // conversation on whether the API should be requiring the domain name to be present.
    params: ['pente', id],
  };
  return <Promise<IPrivacyGroup>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingPrivacyGroup')
    )
  );
};

export const getPrivacyGroupByAddress = async (address: string): Promise<IPrivacyGroup> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_getGroupByAddress,
    params: [address],
  };
  return <Promise<IPrivacyGroup>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingPrivacyGroup')
    )
  );
};

export const buildPrivacyGroupMessagePagingReference = (
  message: IPrivacyGroupMessage,
): ISortPagingReference => ({
  sortValue: message.sent,
  tiebreaker: message.id,
});

export const getPrivacyGroupMessages = async (
  limit: number,
  filters: IFilter[],
  sortAscending: boolean,
  pageRef?: ISortPagingReference,
  privacyGroupId?: string
): Promise<IPagedResult<IPrivacyGroupMessage>> => {

  let translatedFilters = translateFilters(filters);
  const sortDirection = sortAscending ? 'ASC' : 'DESC';

  let customFilters: any = {};
  if (privacyGroupId !== undefined) {
    customFilters.equal = [{
      field: 'group',
      value: privacyGroupId
    }];
  }

  let queryParams: any = {
    ...deepMerge(translatedFilters, customFilters),
    limit: limit + 1,
    sort: [
      `sent ${sortDirection}`,
      `id ${sortDirection}`,
    ],
  };

  if (pageRef !== undefined) {
    const comparison = sortAscending ? 'greaterThan' : 'lessThan';
    queryParams.or = [
      {
        [comparison]: [{
          field: 'sent',
          value: pageRef.sortValue,
        }],
      },
      {
        equal: [{
          field: 'sent',
          value: pageRef.sortValue,
        }],
        [comparison]: [{
          field: 'id',
          value: pageRef.tiebreaker,
        }],
      },
    ];
  }

  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_queryMessages,
    params: [queryParams],
  };
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
    i18next.t('errorFetchingPrivacyGroupMessages')
  );
  return toPagedResult(results, limit);
};

export const getPrivacyGroupMessage = async (
  messageId: string,
  privacyGroupId?: string
): Promise<IPrivacyGroupMessage | null> => {
  let params: any = [
    {
      equal: [{
        field: 'id',
        value: messageId
      }],
      limit: 1
    }
  ]
  if (privacyGroupId !== undefined) {
    params[0].equals.push({
      field: 'group',
      value: privacyGroupId
    });
  }
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_queryMessages,
    params
  };
  const response = await <Promise<IPrivacyGroupMessage[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingPrivacyGroupMessage')
    )
  );
  if (response.length === 1) {
    return response[0];
  }
  return null;
};

export const createPrivacyGroup = async (
  name: string,
  members: string[]
): Promise<IPrivacyGroup> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_createGroup,
    params: [{
      domain: 'pente',
      name,
      members
    }],
  };
  return <Promise<IPrivacyGroup>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingPrivacyGroup')
    )
  );
};

export const sendPrivacyGroupMessage = async (
  group: string,
  topic: string,
  data: any,
  correlationId?: string
): Promise<string> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_sendMessage,
    params: [{
      domain: 'pente',
      group,
      topic,
      data,
      correlationId
    }],
  };
  return <Promise<string>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingPrivacyGroup')
    )
  );
};

export const getPrivacyGroupListenerSortValue = (
  listener: IPrivacyGroupListener,
  sortBy: string,
): any => sortBy === 'created' ? listener.created : listener.name;

export const buildPrivacyGroupListenerPagingReference = (
  listener: IPrivacyGroupListener,
  sortBy: string,
): ISortPagingReference => ({
  sortValue: getPrivacyGroupListenerSortValue(listener, sortBy),
  tiebreaker: listener.name,
});

export const listPrivacyGroupListeners = async (
  limit: number,
  filters: IFilter[],
  sortBy: string,
  sortAscending: boolean,
  pageRef?: ISortPagingReference
): Promise<IPagedResult<IPrivacyGroupListener>> => {
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
    method: RpcMethods.pgroup_queryMessageListeners,
    params: [queryParams]
  };
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
    i18next.t('errorFetchingPrivacyGroups')
  );
  return toPagedResult(results, limit);
};

export const startPrivacyGroupListener = async (
  listenerName: string
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_startMessageListener,
    params: [listenerName],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorStartingPrivacyGroupListener')
    )
  );
};

export const stopPrivacyGroupListener = async (
  listenerName: string
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_stopMessageListener,
    params: [listenerName],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorStoppingPrivacyGroupListener')
    )
  );
};

export const getPrivacyGroupListener = async (
  listenerName: string
): Promise<IPrivacyGroupListener> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_getMessageListener,
    params: [listenerName],
  };
  return <Promise<IPrivacyGroupListener>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorStoppingPrivacyGroupListener')
    )
  );
};

export const createPrivacyGroupListener = async (
  name: string,
  started: boolean,
  filters: IPrivacyGroupMessageListenerFilters,
  options: IPrivacyGroupMessageListenerOptions
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_createMessageListener,
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
      i18next.t('errorDeletingPrivacyGroupListener')
    )
  );
};

export const deletePrivacyGroupListener = async (
  listenerName: string
): Promise<boolean> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_deleteMessageListener,
    params: [listenerName],
  };
  return <Promise<boolean>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorDeletingPrivacyGroupListener')
    )
  );
};