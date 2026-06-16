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

import { IFilter, IMessage, ITransportPeer } from "../interfaces";
import { translateFilters } from "../utils";
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

export const fetchTransportPeers = async (): Promise<ITransportPeer[]> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.transport_peers,
  };

  return <Promise<ITransportPeer[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingTransportPeers")
    )
  );
};

export const queryMessages = async (
  limit: number,
  sortAscending: boolean,
  filters: IFilter[],
  refTimestamp?: string
): Promise<IMessage[]> => {

  let translatedFilters = translateFilters(filters);
  
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.transport_queryReliableMessages,
    params: [{
      ...translatedFilters,
      limit,
      sort: [`created ${sortAscending ? 'ASC' : 'DESC'}`],
      greaterThan: refTimestamp !== undefined && sortAscending ? [
        {
          field: 'created',
          value: refTimestamp
        }
      ] : undefined,
      lessThan: refTimestamp !== undefined && !sortAscending ? [
        {
          field: 'created',
          value: refTimestamp
        }
      ] : undefined
    }]
  };

  return <Promise<IMessage[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingMessages")
    )
  );
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
