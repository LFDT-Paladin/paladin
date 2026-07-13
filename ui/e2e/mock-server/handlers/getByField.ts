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

import { compareValues, hexVariants } from '../query/compareValues.js';
import { getFieldMap, loadCollection } from '../store/dataStore.js';
import type { MethodConfig } from './registry.js';

const valuesMatch = (
  itemValue: unknown,
  paramValue: unknown,
  field: string,
  collection: string
): boolean => {
  const fieldType = getFieldMap(collection)[field] ?? 'string';

  if (fieldType === 'hex') {
    const paramVariants = new Set(hexVariants(paramValue));
    return hexVariants(itemValue).some((variant) => paramVariants.has(variant));
  }

  return compareValues(itemValue, paramValue, fieldType) === 0;
};

export const handleGetByField = (
  config: MethodConfig,
  params: unknown[]
): unknown => {
  const collection = config.collection;
  const field = config.field;
  if (collection === undefined || field === undefined) {
    return null;
  }

  const paramValue = params[config.paramIndex ?? 0];
  const items = loadCollection(collection);
  const match = items.find((item) => valuesMatch(item[field], paramValue, field, collection));

  if (match === undefined) {
    return null;
  }

  if (config.returnField !== undefined) {
    return match[config.returnField] ?? null;
  }

  return match;
};
