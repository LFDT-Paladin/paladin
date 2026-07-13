import { formatUuid } from './format-utils.js';
import { formatSchemaId, formatStateContractAddress, formatStateId } from './state-data.js';

export const TRANSPORT_PEER_COUNT = 25;
export const TRANSPORT_MESSAGE_COUNT = 50;

export const MESSAGE_TYPES = [
  'state',
  'receipt',
  'public_transaction_submission',
  'sequencing_activity',
  'prepared_txn',
  'privacy_group',
  'privacy_group_message',
] as const;

export type MessageType = (typeof MESSAGE_TYPES)[number];

/** Zero-padded peer names for predictable name sort (node01 first in ASC). */
export const formatPeerName = (n: number): string =>
  `node${n.toString(10).padStart(2, '0')}`;

/** Reliable message UUIDs use leading digit `3` (1=PG messages, 2=genesis txs). */
export const formatReliableMessageId = (n: number): string => formatUuid(n, '3');

export interface MockTransportPeer {
  name: string;
  activated: string;
  outboundTransport: string;
  outbound: {
    endpoint: string;
  };
  stats: {
    createdAt: string;
    sentMsgs: number;
    receivedMsgs: number;
    sentBytes: number;
    receivedBytes: number;
    lastSend: string;
    lastReceive: string | null;
    reliableHighestSent: number;
  };
}

export interface MockReliableMessage {
  sequence: number;
  id: string;
  created: string;
  node: string;
  messageType: MessageType;
  metadata: Record<string, string>;
  ack?: {
    time: string;
    error?: string;
  };
}

const descendingCreated = (n: number): string =>
  new Date(Date.UTC(2026, 0, 1, 12, 0, 0, 1000 - n)).toISOString();

/**
 * Builds transport peers for the Transports Connections page.
 *
 * Layout (n=1..25):
 * - name: node01..node25 (zero-padded)
 * - every 5th peer: lastReceive null (shows as --)
 *
 * Conventions:
 * - activated / createdAt: descending from 2026-01-01T12:00:00Z
 * - outboundTransport: grpc
 * - endpoint: dns:///nodeNN.example:18888
 * - sentMsgs: n * 100, receivedMsgs: n * 90
 */
export const buildTransportPeers = (): MockTransportPeer[] => {
  const peers: MockTransportPeer[] = [];

  for (let i = 0; i < TRANSPORT_PEER_COUNT; i++) {
    const n = i + 1;
    const name = formatPeerName(n);
    const activated = descendingCreated(n);
    peers.push({
      name,
      activated,
      outboundTransport: 'grpc',
      outbound: {
        endpoint: `dns:///${name}.example:18888`,
      },
      stats: {
        createdAt: activated,
        sentMsgs: n * 100,
        receivedMsgs: n * 90,
        sentBytes: n * 1024,
        receivedBytes: n * 900,
        lastSend: new Date(Date.parse(activated) + 1000).toISOString(),
        lastReceive: n % 5 === 0 ? null : new Date(Date.parse(activated) + 500).toISOString(),
        reliableHighestSent: n * 100,
      },
    });
  }

  return peers;
};

/**
 * Builds reliable transport messages for the Transports Messages page.
 *
 * Layout (m=1..50, newest-first by created):
 * - node: cycles through peer names
 * - messageType: cycles through MESSAGE_TYPES
 * - every 5th message omits ack (shows as --)
 *
 * Conventions:
 * - id: UUID with leading digit 3
 * - sequence: m
 * - created: descending from 2026-01-01T12:00:00Z
 */
export const buildTransportMessages = (
  peers: MockTransportPeer[]
): MockReliableMessage[] => {
  const messages: MockReliableMessage[] = [];

  for (let i = 0; i < TRANSPORT_MESSAGE_COUNT; i++) {
    const m = i + 1;
    const peer = peers[(m - 1) % peers.length];
    const messageType = MESSAGE_TYPES[(m - 1) % MESSAGE_TYPES.length];
    const created = descendingCreated(m);
    const message: MockReliableMessage = {
      sequence: m,
      id: formatReliableMessageId(m),
      created,
      node: peer.name,
      messageType,
      metadata:
        messageType === 'state'
          ? {
              stateId: formatStateId(((m - 1) % 25) + 1),
              identityLocator: `alice@${peer.name}`,
              domain: 'noto',
              contractAddress: formatStateContractAddress(((m - 1) % 25) + 1),
              schemaId: formatSchemaId(1),
            }
          : {
              node: peer.name,
              messageType,
            },
    };
    if (m % 5 !== 0) {
      message.ack = {
        time: new Date(Date.parse(created) + 1000).toISOString(),
      };
    }
    messages.push(message);
  }

  return messages;
};
