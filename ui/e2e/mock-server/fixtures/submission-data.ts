import { formatHex, formatUuid } from './format-utils.js';

export const PENDING_SUBMISSION_COUNT = 25;
export const FAILED_SUBMISSION_COUNT = 5;
export const SUBMISSION_COUNT = PENDING_SUBMISSION_COUNT + FAILED_SUBMISSION_COUNT;

export const formatSubmissionId = (n: number): string => formatUuid(n);
export const formatSubmissionFrom = (n: number): string => formatHex(n, 40, '1');
export const formatSubmissionTo = (n: number): string => formatHex(n, 40, '2');
export const formatAbiReference = (n: number): string => formatHex(n, 64);
export const formatSubmissionTxHash = (n: number): string => formatHex(n, 64);

export type SubmissionStatus = 'pending' | 'failed';

export interface MockPaladinTransaction {
  id: string;
  created: string;
  type: string;
  domain?: string;
  function: string;
  to?: string;
  from: string;
  abiReference: string;
  data: Record<string, string>;
  /** Present only for failed submissions — used by ptx_queryTransactionsFull filters. */
  success?: boolean;
  receipt?: {
    blockNumber: number;
    domain?: string;
    id: string;
    success: boolean;
    transactionHash: string;
    failureMessage?: string;
  };
}

/**
 * Status for submissions (1-indexed position n):
 * - n = 1..25 → pending (no success / receipt)
 * - n = 26..30 → failed (success: false)
 */
export const submissionStatusForIndex = (n: number): SubmissionStatus =>
  n <= PENDING_SUBMISSION_COUNT ? 'pending' : 'failed';

/**
 * Builds Paladin transactions for the Submissions page (25 pending, 5 failed).
 *
 * Conventions (n = 1..30, newest first by created):
 * - id:        00000000-0000-1000-8000-4...n (pending/failed submission ids)
 * - from:      0x5...n
 * - to:        0x6...n
 * - created:   descending from 2026-01-01T12:00:00Z (n=1 newest)
 * - n 1..25 pending / n 26..30 failed
 */
export const buildPaladinTransactions = (): MockPaladinTransaction[] => {
  const transactions: MockPaladinTransaction[] = [];

  for (let i = 0; i < SUBMISSION_COUNT; i++) {
    const n = i + 1;
    const status = submissionStatusForIndex(n);
    // Newest first: n=1 is most recent
    const created = new Date(Date.UTC(2026, 0, 1, 12, 0, 0, 1000 - n)).toISOString();

    const base: MockPaladinTransaction = {
      id: formatSubmissionId(n),
      created,
      type: 'private',
      domain: 'pente',
      function: 'deploy',
      from: formatSubmissionFrom(n),
      to: formatSubmissionTo(n),
      abiReference: formatAbiReference(n),
      data: {
        amount: String(n * 100),
      },
    };

    if (status === 'pending') {
      transactions.push(base);
      continue;
    }

    transactions.push({
      ...base,
      success: false,
      receipt: {
        id: formatSubmissionId(n),
        blockNumber: 1000 - n,
        success: false,
        transactionHash: formatSubmissionTxHash(n),
        domain: 'pente',
        failureMessage: `submission ${n} failed`,
      },
    });
  }

  return transactions;
};
