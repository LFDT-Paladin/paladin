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

import { loadCollection } from '../store/dataStore.js';
import type { MethodConfig } from './registry.js';

/**
 * Returns an array of a single field from every item in a collection.
 * Used for APIs like domain_listDomains that return name strings, not objects.
 */
export const handleListField = (config: MethodConfig): unknown => {
  const collection = config.collection;
  const field = config.field;
  if (collection === undefined || field === undefined) {
    return [];
  }

  return loadCollection(collection).map((item) => item[field] ?? null);
};
