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

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export interface PreFilterConfig {
  /** Keep items whose sourceField value exists in the related collection's joinField. */
  hasReceiptIn?: string;
  joinField?: string;
  sourceField?: string;
  /** Keep items where this field is null or undefined (e.g. pending submissions). */
  fieldMissing?: string;
  /** Keep items where item[equalField] === params[equalParamIndex]. */
  equalField?: string;
  equalParamIndex?: number;
  /** Multiple equalField prefilters (e.g. domain + schema for pstate_queryStates). */
  equalFields?: Array<{ field: string; paramIndex: number }>;
  /** Apply active/inactive/any filter from params[activeParamIndex]. */
  activeParamIndex?: number;
}

export interface MethodConfig {
  type: 'query' | 'derivedQuery' | 'getByField' | 'listField' | 'static' | 'empty';
  collection?: string;
  queryParamIndex?: number;
  field?: string;
  paramIndex?: number;
  returnField?: string;
  file?: string;
  returnValue?: unknown;
  preFilter?: PreFilterConfig;
}

const rootDir = dirname(fileURLToPath(import.meta.url));

let methodRegistry: Record<string, MethodConfig> | null = null;

export const getMethodRegistry = (): Record<string, MethodConfig> => {
  if (methodRegistry === null) {
    methodRegistry = JSON.parse(
      readFileSync(join(rootDir, '../methods.json'), 'utf-8')
    ) as Record<string, MethodConfig>;
  }
  return methodRegistry;
};

export const getMethodConfig = (method: string): MethodConfig | undefined =>
  getMethodRegistry()[method];
