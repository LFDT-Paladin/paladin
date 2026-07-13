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

import { formatHex } from './format-utils.js';

export const NOTO_COIN_STATE_COUNT = 25;
export const NOTO_LOCKED_COIN_STATE_COUNT = 5;
export const ZETO_COIN_STATE_COUNT = 10;
export const PENTE_ACCOUNT_STATE_COUNT = 5;
export const STATE_COUNT =
  NOTO_COIN_STATE_COUNT +
  NOTO_LOCKED_COIN_STATE_COUNT +
  ZETO_COIN_STATE_COUNT +
  PENTE_ACCOUNT_STATE_COUNT;

/** Schema IDs use leading nibble `8` (bytes32). */
export const formatSchemaId = (n: number): string => formatHex(n, 64, '8');
/** State IDs use leading nibble `7` (bytes32). */
export const formatStateId = (n: number): string => formatHex(n, 64, '7');
/** State contract addresses use leading nibble `9`. */
export const formatStateContractAddress = (n: number): string => formatHex(n, 40, '9');
/** State owner addresses use leading nibble `c`. */
export const formatStateOwner = (n: number): string => formatHex(n, 40, 'c');
/** State salt values use leading nibble `d`. */
export const formatStateSalt = (n: number): string => formatHex(n, 64, 'd');

export const SCHEMA_IDS = {
  notoCoin: formatSchemaId(1),
  notoLockedCoin: formatSchemaId(2),
  zetoCoin: formatSchemaId(3),
  penteAccount: formatSchemaId(4),
} as const;

export interface MockSchemaComponent {
  name: string;
  type: string;
  indexed: boolean;
}

export interface MockSchema {
  id: string;
  created: string;
  domain: string;
  type: string;
  signature: string;
  definition: {
    name: string;
    type: string;
    internalType: string;
    components: MockSchemaComponent[];
  };
  labels: string[];
}

export interface MockState {
  id: string;
  created: string;
  domain: string;
  schema: string;
  contractAddress: string | null;
  data: Record<string, unknown>;
}

const descendingCreated = (n: number): string =>
  new Date(Date.UTC(2026, 0, 1, 12, 0, 0, 1000 - n)).toISOString();

const buildSchema = (
  id: string,
  domain: string,
  name: string,
  components: MockSchemaComponent[],
  createdMs: number
): MockSchema => {
  const labels = components.filter((c) => c.indexed).map((c) => c.name);
  const signatureParts = components
    .map((c) => `${c.type} ${c.name}`)
    .join(',');
  return {
    id,
    created: new Date(Date.UTC(2026, 0, 1, 12, 0, 0, createdMs)).toISOString(),
    domain,
    type: 'abi',
    signature: `type=${name}(${signatureParts}),labels=[${labels.join(',')}]`,
    definition: {
      name,
      type: 'tuple',
      internalType: `struct ${name}`,
      components,
    },
    labels,
  };
};

/**
 * Builds schemas for the States page domain/schema selectors.
 *
 * Layout (order matters within each domain — UI auto-selects schemas[0]):
 * - noto:  NotoCoin (default), NotoLockedCoin
 * - zeto:  ZetoCoin
 * - pente: AccountState
 */
export const buildSchemas = (): MockSchema[] => [
  buildSchema(
    SCHEMA_IDS.notoCoin,
    'noto',
    'NotoCoin',
    [
      { name: 'salt', type: 'bytes32', indexed: false },
      { name: 'owner', type: 'address', indexed: true },
      { name: 'amount', type: 'uint256', indexed: true },
    ],
    1
  ),
  buildSchema(
    SCHEMA_IDS.notoLockedCoin,
    'noto',
    'NotoLockedCoin',
    [
      { name: 'salt', type: 'bytes32', indexed: false },
      { name: 'owner', type: 'address', indexed: true },
      { name: 'amount', type: 'uint256', indexed: true },
      { name: 'locked', type: 'bool', indexed: true },
    ],
    2
  ),
  buildSchema(
    SCHEMA_IDS.zetoCoin,
    'zeto',
    'ZetoCoin',
    [
      { name: 'salt', type: 'bytes32', indexed: false },
      { name: 'owner', type: 'bytes32', indexed: true },
      { name: 'amount', type: 'uint256', indexed: true },
      { name: 'locked', type: 'bool', indexed: true },
    ],
    3
  ),
  buildSchema(
    SCHEMA_IDS.penteAccount,
    'pente',
    'AccountState',
    [
      { name: 'address', type: 'address', indexed: true },
      { name: 'balance', type: 'uint256', indexed: true },
    ],
    4
  ),
];

const buildState = (
  sequence: number,
  domain: string,
  schemaId: string,
  data: Record<string, unknown>,
  includeContractAddress: boolean
): MockState => ({
  id: formatStateId(sequence),
  created: descendingCreated(sequence),
  domain,
  schema: schemaId,
  contractAddress: includeContractAddress
    ? formatStateContractAddress(sequence)
    : null,
  data,
});

/**
 * Builds private states for the States page.
 *
 * Layout (global id sequence, newest-first by created within each schema):
 * - n=1..25  → noto / NotoCoin
 * - n=26..30 → noto / NotoLockedCoin
 * - n=31..40 → zeto / ZetoCoin
 * - n=41..45 → pente / AccountState
 *
 * Conventions:
 * - id:              0x7...n
 * - contractAddress: 0x9...n (null every 5th state)
 * - owner:           0xc...n (address) or 0xd...n (bytes32 for zeto)
 * - amount/balance:  n * 100
 * - created:         descending from 2026-01-01T12:00:00Z
 */
export const buildStates = (): MockState[] => {
  const states: MockState[] = [];
  let sequence = 1;

  for (let i = 0; i < NOTO_COIN_STATE_COUNT; i++) {
    const local = i + 1;
    const n = sequence;
    states.push(
      buildState(
        sequence++,
        'noto',
        SCHEMA_IDS.notoCoin,
        {
          salt: formatStateSalt(n),
          owner: formatStateOwner(n),
          amount: local * 100,
        },
        n % 5 !== 0
      )
    );
  }

  for (let i = 0; i < NOTO_LOCKED_COIN_STATE_COUNT; i++) {
    const local = i + 1;
    const n = sequence;
    states.push(
      buildState(
        sequence++,
        'noto',
        SCHEMA_IDS.notoLockedCoin,
        {
          salt: formatStateSalt(n),
          owner: formatStateOwner(n),
          amount: local * 50,
          locked: local % 2 === 1,
        },
        n % 5 !== 0
      )
    );
  }

  for (let i = 0; i < ZETO_COIN_STATE_COUNT; i++) {
    const local = i + 1;
    const n = sequence;
    states.push(
      buildState(
        sequence++,
        'zeto',
        SCHEMA_IDS.zetoCoin,
        {
          salt: formatStateSalt(n),
          owner: formatStateSalt(n), // bytes32 owner
          amount: local * 25,
          locked: local % 2 === 0,
        },
        n % 5 !== 0
      )
    );
  }

  for (let i = 0; i < PENTE_ACCOUNT_STATE_COUNT; i++) {
    const local = i + 1;
    const n = sequence;
    states.push(
      buildState(
        sequence++,
        'pente',
        SCHEMA_IDS.penteAccount,
        {
          address: formatStateOwner(n),
          balance: local * 1000,
        },
        n % 5 !== 0
      )
    );
  }

  return states;
};

export const SCHEMA_COUNT = 4;
