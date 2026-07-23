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

import { loadStatic } from '../store/dataStore.js';
import { handleDerivedQuery } from './derivedQuery.js';
import { handleGetByField } from './getByField.js';
import { handleListField } from './listField.js';
import { handleQueryList } from './queryList.js';
import { getMethodConfig } from './registry.js';
import type { MethodConfig } from './registry.js';

export const handleRpcMethod = async (
  method: string,
  params: unknown[] = []
): Promise<unknown> => {
  const config = getMethodConfig(method);
  if (config === undefined) {
    console.warn(`[mock-rpc] unhandled method: ${method}, returning []`);
    return [];
  }

  return dispatchMethod(config, params);
};

const dispatchMethod = (config: MethodConfig, params: unknown[]): unknown => {
  switch (config.type) {
    case 'query':
      return handleQueryList(config, params);
    case 'derivedQuery':
      return handleDerivedQuery(config, params);
    case 'getByField':
      return handleGetByField(config, params);
    case 'listField':
      return handleListField(config);
    case 'static':
      return config.file !== undefined ? loadStatic(config.file) : config.returnValue ?? null;
    case 'empty':
      return config.returnValue ?? [];
    default:
      return [];
  }
};
