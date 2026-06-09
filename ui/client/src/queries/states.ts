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
import { IFilter, ISchema, IState, IStateReceipt } from "../interfaces";
import { generatePostReq, returnResponse } from "./common";
import { RpcEndpoint, RpcMethods } from "./rpcMethods";
import { translateFilters } from "../utils";

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
  refTimestamp?: string
): Promise<IState[]> => {

  let translatedFilters = translateFilters(filters);

  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.pstate_queryStates,
    params: [
      domain,
      schemaId,
      {
        ...translatedFilters,
        limit,
        sort: [`${sortBy} ${sortAscending ? 'ASC' : 'DESC'}`],
        greaterThan: refTimestamp !== undefined && sortAscending ? [
          {
            field: '.created',
            value: refTimestamp
          }
        ] : undefined,
        lessThan: refTimestamp !== undefined && !sortAscending ? [
          {
            field: '.created',
            value: refTimestamp
          }
        ] : undefined
      },
      'all'
    ]
  };
  return <Promise<IState[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingSchemas"), []
    )
  );
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
      i18next.t("errorFetchingSchemas"), []
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
    method: RpcMethods.pstate_transferState,
    params: [
      domain,
      stateId,
      recipient
    ]
  };
  return <Promise<string>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingSchemas"), []
    )
  );
};

