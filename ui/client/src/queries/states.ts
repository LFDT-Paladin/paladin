// Copyright © 2024 Kaleido, Inc.
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

import i18next from "i18next";
import { IFilter, IPagedResult, ISchema, IState, IStatePagingReference, IStateReceipt } from "../interfaces";
import { generatePostReq, returnResponse } from "./common";
import { RpcEndpoint, RpcMethods } from "./rpcMethods";
import { toPagedResult, translateFilters } from "../utils";

export const getStateSortValue = (state: IState, sortBy: string): any => {
  if (sortBy === '.created') {
    return state.created;
  }
  return state.data[sortBy];
};

export const buildStatePagingReference = (state: IState, sortBy: string): IStatePagingReference => ({
  sortValue: getStateSortValue(state, sortBy),
  id: state.id,
});

export const fetchStateReceipt = async (
  transactionId: string
): Promise<IStateReceipt> => {
  const payload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.ptx_getStateReceipt,
    params: [transactionId],
  };

  return <Promise<IStateReceipt>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingStateReceipt")
    )
  );
};

export const resolveVerifier = async (keyIdentifier: string, algorithm: string, verifierType: string): Promise<string> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.ptx_resolveVerifier,
    params: [keyIdentifier, algorithm, verifierType]
  };

  return <Promise<string>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingResolveVerifier"), []
    )
  );
};

export const listSchemas = async (domain: string): Promise<ISchema[]> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.pstate_listSchemas,
    params: [domain]
  };
  return <Promise<ISchema[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingSchemas"), []
    )
  );
};

export const queryStates = async (
  domain: string,
  schemaId: string,
  limit: number,
  sortBy: string,
  sortAscending: boolean,
  filters: IFilter[],
  pageRef?: IStatePagingReference
): Promise<IPagedResult<IState>> => {

  let translatedFilters = translateFilters(filters);
  const sortDirection = sortAscending ? 'ASC' : 'DESC';

  let queryParams: any = {
    ...translatedFilters,
    limit: limit + 1,
    sort: [
      `${sortBy} ${sortDirection}`,
      `.id ${sortDirection}`,
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
          field: '.id',
          value: pageRef.id,
        }],
      },
    ];
  }

  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.pstate_queryStates,
    params: [
      domain,
      schemaId,
      queryParams,
      'all'
    ]
  };
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
    i18next.t("errorFetchingStates"), []
  );
  return toPagedResult(results, limit);
};

export const getState = async (
  domain: string,
  schemaId: string,
  id: string
): Promise<IState | null> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.pstate_queryStates,
    params: [
      domain,
      schemaId,
      {
        limit: 1,
        "equal": [{
          "field": ".id",
          "value": id
        }]
      },
      'all'
    ]
  };
  const states = await <Promise<IState[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingState"), []
    )
  );
  if (states.length === 0) {
    return null;
  }
  return states[0];
};

export const pushState = async (
  domain: string,
  stateId: string,
  recipient: string
): Promise<string> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.pstate_transferPrivateState,
    params: [
      domain,
      stateId,
      recipient
    ]
  };
  return <Promise<string>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorPushingState"), []
    )
  );
};

