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
import { IFetchRegistryEntriesParams, IPagedResult, IRegistryEntry } from "../interfaces";
import { generatePostReq, returnResponse } from "./common";
import { RpcEndpoint, RpcMethods } from "./rpcMethods";
import { deepMerge, toPagedResult, translateFilters } from "../utils";

export const fetchRegistries = async (): Promise<string[]> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.reg_registries,
  };

  return <Promise<string[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingRegistries")
    )
  );
};

export const fetchRegistryEntries = async (
  params: IFetchRegistryEntriesParams
): Promise<IPagedResult<IRegistryEntry>> => {
  const { registryName, filters, tab, limit, pageParam, sortAscending, excludeRoot } = params;
  const translatedFilters = translateFilters(filters);
  let customFilters: any = {};
  if(excludeRoot === true) {
    customFilters.neq = [{
      field: '.name',
      value: 'root'
    }]
  }

  let requestPayload: any = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.reg_queryEntriesWithProps,
    params: [
      registryName,
      {
        ...deepMerge(translatedFilters, customFilters),
        limit: limit + 1,
        sort: [`.name ${sortAscending ? 'ASC' : 'DESC'}`]
      },
      tab
    ]
  };
  if (pageParam !== undefined) {
    requestPayload.params[1].greaterThan = [
      {
        "field": ".name",
        "value": pageParam
      }
    ];
  }
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
    i18next.t("errorFetchingRegistryEntries")
  );
  return toPagedResult(results, limit);
};

export const fetchRegistryEntry = async (
  registryName: string,
  id: string
) => {
  let requestPayload: any = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.reg_queryEntriesWithProps,
    params: [
      registryName,
      {
        equal: [{
          field: '.id',
          value: id
        }],
        limit: 1
      },
      'all'
    ]
  };
  const result = await <Promise<IRegistryEntry[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingRegistryEntry")
    )
  );
  if(result.length === 1) {
    return result[0];
  }
  return null;
};