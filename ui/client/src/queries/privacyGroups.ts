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
import { IFilter, IPrivacyGroup } from '../interfaces';
import { translateFilters } from '../utils';

export const listPrivacyGroups = async (
  limit: number,
  filters: IFilter[],
  sortAscending: boolean,
  refTimestamp?: string
): Promise<IPrivacyGroup[]> => {
  let translatedFilters = translateFilters(filters);

  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.pgroup_queryGroups,
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
  return <Promise<IPrivacyGroup[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingPrivacyGroups')
    )
  );
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