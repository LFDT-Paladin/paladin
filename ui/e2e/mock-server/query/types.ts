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

export type FieldType = 'string' | 'int' | 'float' | 'bool' | 'hex' | 'timestamp';

export type FieldMap = Record<string, FieldType>;

export interface Op {
  not?: boolean;
  caseInsensitive?: boolean;
  field?: string;
}

export interface OpSingleVal extends Op {
  value?: unknown;
}

export interface OpMultiVal extends Op {
  values?: unknown[];
}

export interface Statements {
  or?: Statements[];
  equal?: OpSingleVal[];
  eq?: OpSingleVal[];
  neq?: OpSingleVal[];
  like?: OpSingleVal[];
  lessThan?: OpSingleVal[];
  lt?: OpSingleVal[];
  lessThanOrEqual?: OpSingleVal[];
  lte?: OpSingleVal[];
  greaterThan?: OpSingleVal[];
  gt?: OpSingleVal[];
  greaterThanOrEqual?: OpSingleVal[];
  gte?: OpSingleVal[];
  in?: OpMultiVal[];
  nin?: OpMultiVal[];
  null?: Op[];
}

export interface QueryJSON extends Statements {
  limit?: number;
  sort?: string[];
}

export interface SortField {
  field: string;
  descending: boolean;
}
