import { formatHex, formatUuid } from './format-utils.js';

export const TRANSACTION_COUNT = 100;

export const formatTxHash = (n: number): string => formatHex(n, 64);
export const formatAddress = (n: number): string => formatHex(n, 40);
export const formatFromAddress = (n: number): string => formatHex(n, 40, '1');
export const formatToAddress = (n: number): string => formatHex(n, 40, '2');
export const formatReceiptId = (n: number): string => formatUuid(n);
export const formatEventHash = (n: number): string => formatHex(n, 64, '3');
export const formatBlockHash = (blockNumber: number): string =>
  formatHex(blockNumber, 64, '4');

export interface MockBlock {
  number: number;
  hash: string;
  timestamp: string;
}

export interface MockTransaction {
  hash: string;
  blockNumber: number;
  transactionIndex: number;
  from: string;
  to: string;
  nonce: number;
  result: string;
  block: MockBlock;
}

export interface MockTransactionReceipt {
  blockNumber: number;
  domain: string;
  id: string;
  success: boolean;
  transactionHash: string;
}

export interface MockEvent {
  blockNumber: number;
  transactionIndex: number;
  logIndex: number;
  transactionHash: string;
  signature: string;
  block: MockBlock;
}

const buildBlock = (blockNumber: number): MockBlock => ({
  number: blockNumber,
  hash: formatBlockHash(blockNumber),
  timestamp: new Date(Date.UTC(2026, 0, 1, 12, 0, blockNumber)).toISOString(),
});

export const buildTransactions = (): MockTransaction[] => {
  const transactions: MockTransaction[] = [];

  for (let i = 0; i < TRANSACTION_COUNT; i++) {
    const n = i + 1;
    const blockNumber = 100 - i;

    transactions.push({
      hash: formatTxHash(n),
      blockNumber,
      transactionIndex: 0,
      from: formatFromAddress(n),
      to: formatToAddress(n),
      nonce: n,
      result: 'success',
      block: buildBlock(blockNumber),
    });
  }

  return transactions;
};

export const buildReceipts = (transactions: MockTransaction[]): MockTransactionReceipt[] => {
  const receipts: MockTransactionReceipt[] = [];
  let receiptCounter = 1;

  for (let i = 0; i < TRANSACTION_COUNT; i++) {
    if (i % 2 !== 0) {
      continue;
    }

    const tx = transactions[i];
    for (let receiptIndex = 0; receiptIndex < 2; receiptIndex++) {
      receipts.push({
        id: formatReceiptId(receiptCounter++),
        blockNumber: tx.blockNumber,
        success: true,
        transactionHash: tx.hash,
        domain: 'pente',
      });
    }
  }

  return receipts;
};

export const buildEvents = (transactions: MockTransaction[]): MockEvent[] => {
  const events: MockEvent[] = [];
  let eventHashCounter = 1;

  for (let i = 0; i < TRANSACTION_COUNT; i++) {
    if (i % 2 !== 0) {
      continue;
    }

    const tx = transactions[i];
    for (let logIndex = 0; logIndex < 2; logIndex++) {
      events.push({
        blockNumber: tx.blockNumber,
        transactionIndex: tx.transactionIndex,
        logIndex,
        transactionHash: tx.hash,
        signature: formatEventHash(eventHashCounter++),
        block: tx.block,
      });
    }
  }

  return events;
};

export const EMPTY_COLLECTIONS = [
  'privacy-groups',
  'privacy-group-messages',
  'privacy-group-listeners',
  'states',
  'transport-peers',
  'transport-messages',
];
