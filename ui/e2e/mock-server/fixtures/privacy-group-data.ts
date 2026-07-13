import { formatHex, formatUuid } from './format-utils.js';

export const PRIVACY_GROUP_COUNT = 25;
export const PRIVACY_GROUP_MESSAGE_COUNT = 50;
export const PRIVACY_GROUP_LISTENER_COUNT = 12;
/** First N listeners are started; the rest are stopped. */
export const PRIVACY_GROUP_LISTENER_STARTED_COUNT = 8;

/** Group IDs use leading nibble `e` (bytes32). */
export const formatPrivacyGroupId = (n: number): string => formatHex(n, 64, 'e');
/** Contract addresses use leading nibble `4`. */
export const formatPrivacyGroupAddress = (n: number): string => formatHex(n, 40, '4');
export const formatGenesisSalt = (n: number): string => formatHex(n, 64, '5');
export const formatGenesisSchema = (n: number): string => formatHex(n, 64, '6');
/** Message UUIDs use leading digit `1` (distinct from receipt/submission ids). */
export const formatMessageId = (n: number): string => formatUuid(n, '1');
/** Genesis transaction UUIDs use leading digit `2`. */
export const formatGenesisTxId = (n: number): string => formatUuid(n, '2');

export interface MockPrivacyGroup {
  id: string;
  domain: string;
  created: string;
  name: string;
  members: string[];
  properties: Record<string, string>;
  configuration: {
    endorsementType: string;
    evmVersion: string;
    externalCallsEnabled: boolean;
  };
  genesisSalt: string;
  genesisSchema: string;
  genesisTransaction: string;
  contractAddress: string;
}

export interface MockPrivacyGroupMessage {
  id: string;
  localSequence: number;
  sent: string;
  received?: string;
  node: string;
  domain: string;
  group: string;
  topic: string;
  data: Record<string, string>;
}

export interface MockPrivacyGroupListener {
  name: string;
  created: string;
  started: boolean;
  filters?: {
    sequenceAbove?: number;
    domain?: string;
    group?: string;
    topic?: string;
  };
  options?: {
    excludeLocal?: boolean;
  };
}

const padIndex = (n: number): string => n.toString(10).padStart(2, '0');

const descendingCreated = (n: number): string =>
  new Date(Date.UTC(2026, 0, 1, 12, 0, 0, 1000 - n)).toISOString();

/**
 * Builds privacy groups for the Privacy Groups page.
 *
 * Layout (n=1..25, newest-first by created):
 * - name: group01..group25 (zero-padded for predictable name sort)
 * - domain: pente
 * - members: alice@node1, bob@node2 (plus carol@node3 on odd n)
 *
 * Conventions:
 * - id:              0xe...n
 * - contractAddress: 0x4...n
 * - genesisSalt:     0x5...n
 * - genesisSchema:   0x6...n
 * - genesisTransaction: UUID with leading digit 2
 * - created:         descending from 2026-01-01T12:00:00Z
 */
export const buildPrivacyGroups = (): MockPrivacyGroup[] => {
  const groups: MockPrivacyGroup[] = [];

  for (let i = 0; i < PRIVACY_GROUP_COUNT; i++) {
    const n = i + 1;
    const members = ['alice@node1', 'bob@node2'];
    if (n % 2 === 1) {
      members.push('carol@node3');
    }

    groups.push({
      id: formatPrivacyGroupId(n),
      domain: 'pente',
      created: descendingCreated(n),
      name: `group${padIndex(n)}`,
      members,
      properties: {},
      configuration: {
        endorsementType: 'group',
        evmVersion: 'shanghai',
        externalCallsEnabled: true,
      },
      genesisSalt: formatGenesisSalt(n),
      genesisSchema: formatGenesisSchema(n),
      genesisTransaction: formatGenesisTxId(n),
      contractAddress: formatPrivacyGroupAddress(n),
    });
  }

  return groups;
};

/**
 * Builds privacy group messages (2 per group by default).
 *
 * Layout (m=1..50, newest-first by sent):
 * - group: cycles through privacy group ids
 * - every 5th message omits received (shows as -- in the UI)
 */
export const buildPrivacyGroupMessages = (
  groups: MockPrivacyGroup[]
): MockPrivacyGroupMessage[] => {
  const messages: MockPrivacyGroupMessage[] = [];

  for (let i = 0; i < PRIVACY_GROUP_MESSAGE_COUNT; i++) {
    const m = i + 1;
    const group = groups[(m - 1) % groups.length];
    const sent = descendingCreated(m);
    const message: MockPrivacyGroupMessage = {
      id: formatMessageId(m),
      localSequence: m,
      sent,
      node: m % 2 === 1 ? 'node1' : 'node2',
      domain: 'pente',
      group: group.id,
      topic: m % 3 === 0 ? 'settlement' : 'orders',
      data: { message: `hello-${padIndex(m)}` },
    };
    if (m % 5 !== 0) {
      message.received = new Date(
        Date.parse(sent) + 1000
      ).toISOString();
    }
    messages.push(message);
  }

  return messages;
};

/**
 * Builds privacy group message listeners.
 *
 * Layout (k=1..12):
 * - listener01..listener08 started; listener09..listener12 stopped
 * - first 6 bind filters to a specific group + topic
 */
export const buildPrivacyGroupListeners = (
  groups: MockPrivacyGroup[]
): MockPrivacyGroupListener[] => {
  const listeners: MockPrivacyGroupListener[] = [];

  for (let i = 0; i < PRIVACY_GROUP_LISTENER_COUNT; i++) {
    const k = i + 1;
    const listener: MockPrivacyGroupListener = {
      name: `listener${padIndex(k)}`,
      created: descendingCreated(k),
      started: k <= PRIVACY_GROUP_LISTENER_STARTED_COUNT,
      options: { excludeLocal: k % 2 === 0 },
    };
    if (k <= 6) {
      listener.filters = {
        domain: 'pente',
        group: groups[k - 1].id,
        topic: 'orders',
      };
    }
    listeners.push(listener);
  }

  return listeners;
};
