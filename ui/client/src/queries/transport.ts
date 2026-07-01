// Copyright © 2025 Kaleido, Inc.
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

import { IFilter, IMessage, IPagedResult, ITransportPeer } from "../interfaces";
import { deepMerge, toPagedResult, translateFilters } from "../utils";
import { generatePostReq, returnResponse } from "./common";
import { RpcEndpoint, RpcMethods } from "./rpcMethods";
import i18next from "i18next";

export const fetchTransportNodeName = async (): Promise<string> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.transport_nodeName,
  };
  return <Promise<string>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingTransportNodeName")
    )
  );
};

export const fetchTransportLocalDetails = async (transport: string): Promise<string> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.transport_localTransportDetails,
    params: [transport]
  };
  return <Promise<string>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingTransportLocalDetails")
    )
  );
};

export const fetchTransportPeersWithQuery = async (
  limit: number,
  sortAscending: boolean,
  filters: IFilter[],
  refData?: string
): Promise<IPagedResult<ITransportPeer>> => {
  let translatedFilters = translateFilters(filters);
  let customFilters: any = {};
  if (refData !== undefined) {
    if (sortAscending) {
      customFilters.greaterThan = [{
        field: 'name',
        value: refData
      }];
    } else {
      customFilters.lessThan = [{
        field: 'name',
        value: refData
      }];
    }
  };
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.transport_queryPeers,
    params: [{
      ...deepMerge(translatedFilters, customFilters),
      limit: limit + 1,
      sort: [`name ${sortAscending ? 'ASC' : 'DESC'}`]
    }]
  };
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
    i18next.t("errorFetchingTransportPeers")
  );
  return toPagedResult(results, limit);
};

export const queryMessages = async (
  limit: number,
  sortBy: string,
  sortAscending: boolean,
  filters: IFilter[],
  refTimestamp?: string
): Promise<IPagedResult<IMessage>> => {
  let translatedFilters = translateFilters(filters);
  let customFilters: any = {};
  if (refTimestamp !== undefined) {
    if (sortAscending) {
      customFilters.greaterThan = [{
        field: sortBy,
        value: refTimestamp
      }];
    } else {
      customFilters.lessThan = [{
        field: sortBy,
        value: refTimestamp
      }];
    }
  };
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.transport_queryReliableMessages,
    params: [{
      ...deepMerge(translatedFilters, customFilters),
      limit: limit + 1,
      sort: [`${sortBy} ${sortAscending ? 'ASC' : 'DESC'}`]
    }]
  };
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
    i18next.t("errorFetchingMessages")
  );
  return toPagedResult(results, limit);
};

export const getMessage = async (
  id: string
): Promise<IMessage | null> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.transport_queryReliableMessages,
    params: [{
      "limit": 1,
      "equal": [{
        "field": "id",
        "value": id
      }]
    }]
  };
  const messages = await <Promise<IMessage[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingMessages")
    )
  );
  if (messages.length === 0) {
    return null;
  }
  return messages[0];
};
