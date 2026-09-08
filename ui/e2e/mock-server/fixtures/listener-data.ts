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

export const EVENT_LISTENER_COUNT = 12;
export const RECEIPT_LISTENER_COUNT = 12;
/** First N listeners of each type are started; the rest are stopped. */
export const LISTENER_STARTED_COUNT = 8;

/** Event source contract addresses use leading nibble `f`. */
export const formatEventSourceAddress = (n: number): string => formatHex(n, 40, 'f');

export const formatEventListenerName = (n: number): string =>
  `eventlistener${n.toString(10).padStart(2, '0')}`;

export const formatReceiptListenerName = (n: number): string =>
  `receiptlistener${n.toString(10).padStart(2, '0')}`;

const TRANSFER_EVENT_ABI = [
  {
    type: 'event',
    name: 'Transfer',
    inputs: [
      { name: 'from', type: 'address', indexed: true },
      { name: 'to', type: 'address', indexed: true },
      { name: 'value', type: 'uint256', indexed: false },
    ],
  },
];

const DOMAINS = ['noto', 'pente', 'zeto'] as const;
const TX_TYPES = ['public', 'private'] as const;
const INCOMPLETE_BEHAVIORS = ['block_contract', 'process', 'complete_only'] as const;

export interface MockEventListener {
  name: string;
  created: string;
  started: boolean;
  sources: Array<{
    abi: typeof TRANSFER_EVENT_ABI;
    address?: string;
  }>;
  options: {
    batchSize: number;
    batchTimeout: string;
    fromBlock: string | number;
  };
}

export interface MockReceiptListener {
  name: string;
  created: string;
  started: boolean;
  filters: {
    sequenceAbove?: number;
    type?: 'public' | 'private';
    domain?: string;
  };
  options: {
    domainReceipts: boolean;
    incompleteStateReceiptBehavior: (typeof INCOMPLETE_BEHAVIORS)[number];
  };
}

const descendingCreated = (n: number): string =>
  new Date(Date.UTC(2026, 0, 1, 12, 0, 0, 1000 - n)).toISOString();

/**
 * Builds blockchain event listeners for the Event Listeners page.
 *
 * Layout (n=1..12):
 * - name: eventlistener01..eventlistener12
 * - first 8 started; last 4 stopped
 * - sources: Transfer event ABI; address on every listener
 * - options.fromBlock: n for most; "latest" on every 4th
 */
export const buildEventListeners = (): MockEventListener[] => {
  const listeners: MockEventListener[] = [];

  for (let i = 0; i < EVENT_LISTENER_COUNT; i++) {
    const n = i + 1;
    listeners.push({
      name: formatEventListenerName(n),
      created: descendingCreated(n),
      started: n <= LISTENER_STARTED_COUNT,
      sources: [
        {
          abi: TRANSFER_EVENT_ABI,
          address: formatEventSourceAddress(n),
        },
      ],
      options: {
        batchSize: 50 + n * 10,
        batchTimeout: `${n}s`,
        fromBlock: n % 4 === 0 ? 'latest' : n,
      },
    });
  }

  return listeners;
};

/**
 * Builds transaction receipt listeners for the Receipt Listeners page.
 *
 * Layout (n=1..12):
 * - name: receiptlistener01..receiptlistener12
 * - first 8 started; last 4 stopped
 * - filters.domain cycles noto/pente/zeto
 * - filters.type alternates public/private
 */
export const buildReceiptListeners = (): MockReceiptListener[] => {
  const listeners: MockReceiptListener[] = [];

  for (let i = 0; i < RECEIPT_LISTENER_COUNT; i++) {
    const n = i + 1;
    listeners.push({
      name: formatReceiptListenerName(n),
      created: descendingCreated(n),
      started: n <= LISTENER_STARTED_COUNT,
      filters: {
        sequenceAbove: n > 6 ? n * 100 : undefined,
        type: TX_TYPES[(n - 1) % TX_TYPES.length],
        domain: DOMAINS[(n - 1) % DOMAINS.length],
      },
      options: {
        domainReceipts: n % 2 === 1,
        incompleteStateReceiptBehavior:
          INCOMPLETE_BEHAVIORS[(n - 1) % INCOMPLETE_BEHAVIORS.length],
      },
    });
  }

  return listeners;
};
