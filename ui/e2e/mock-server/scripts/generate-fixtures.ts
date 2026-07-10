import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  buildEvents,
  buildReceipts,
  buildTransactions,
  EMPTY_COLLECTIONS,
} from '../fixtures/transaction-data.js';
import { buildPaladinTransactions } from '../fixtures/submission-data.js';

const rootDir = dirname(fileURLToPath(import.meta.url));
const dataDir = join(rootDir, '../store/data');
mkdirSync(dataDir, { recursive: true });

const transactions = buildTransactions();
const receipts = buildReceipts(transactions);
const events = buildEvents(transactions);
const paladinTransactions = buildPaladinTransactions();

writeFileSync(
  join(dataDir, 'indexed-transactions.json'),
  `${JSON.stringify(transactions, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'transaction-receipts.json'),
  `${JSON.stringify(receipts, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'indexed-events.json'),
  `${JSON.stringify(events, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'paladin-transactions.json'),
  `${JSON.stringify(paladinTransactions, null, 2)}\n`
);

for (const collection of EMPTY_COLLECTIONS) {
  writeFileSync(join(dataDir, `${collection}.json`), '[]\n');
}

const pendingCount = paladinTransactions.filter((tx) => tx.success === undefined).length;
const failedCount = paladinTransactions.filter((tx) => tx.success === false).length;

console.log(
  `Generated ${transactions.length} indexed transactions, ${receipts.length} receipts, ${events.length} events`
);
console.log(
  `Generated ${paladinTransactions.length} paladin transactions (${pendingCount} pending, ${failedCount} failed)`
);
