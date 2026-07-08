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
import { IDomain, IDomainContract, IFilter, IPagedResult, ISortPagingReference } from '../interfaces';
import { toPagedResult, translateFilters } from '../utils';

export const listDomains = async (): Promise<string[]> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.domain_listDomains,
    params: [],
  };
  const result = await <Promise<string[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingDomains')
    )
  );
  return result.sort();
};

export const getDomainByName = async (name: string): Promise<IDomain> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.domain_getDomain,
    params: [name],
  };

  return <Promise<IDomain>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingDomain')
    )
  );
};

export const buildDomainContractPagingReference = (
  contract: IDomainContract,
): ISortPagingReference => ({
  sortValue: contract.created,
  tiebreaker: contract.address,
});

export const querySmartContractsByDomain = async (
  domainAddress: string,
  sortAscending: boolean,
  rowsPerPage: number,
  filters: IFilter[],
  pageRef?: ISortPagingReference
): Promise<IPagedResult<IDomainContract>> => {
  let translatedFilters = translateFilters(filters);
  const sortDirection = sortAscending ? 'ASC' : 'DESC';

  if(translatedFilters.equal !== undefined) {
    translatedFilters.equal.push({ field: 'domainAddress', value: domainAddress });
  } else {
    translatedFilters.equal = [{ field: 'domainAddress', value: domainAddress }];
  }

  let queryParams: any = {
    ...translatedFilters,
    limit: rowsPerPage + 1,
    sort: [
      `created ${sortDirection}`,
      `address ${sortDirection}`,
    ],
  };

  if (pageRef !== undefined) {
    const comparison = sortAscending ? 'greaterThan' : 'lessThan';
    queryParams.or = [
      {
        [comparison]: [{
          field: 'created',
          value: pageRef.sortValue,
        }],
      },
      {
        equal: [{
          field: 'created',
          value: pageRef.sortValue,
        }],
        [comparison]: [{
          field: 'address',
          value: pageRef.tiebreaker,
        }],
      },
    ];
  }

  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.domain_querySmartContracts,
    params: [queryParams],
  };
  const results = await returnResponse(
    () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
    i18next.t('errorFetchingSmartContracts')
  );
  return toPagedResult(results, rowsPerPage);
};

export const fetchDomainReceipt = async (
  domain: string,
  transactionId: string
): Promise<any> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_getDomainReceipt,
    params: [domain, transactionId],
  };

  return <Promise<any>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingDomainReceipt'), [500]
    )
  );
};

export const getDomainContractByAddress = async (
  address: string,
): Promise<IDomainContract> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.domain_getSmartContractByAddress,
    params: [address],
  };

  return <Promise<IDomainContract>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingDomainContract')
    )
  );
};

