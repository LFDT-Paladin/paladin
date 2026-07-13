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
import { buildKeys } from '../fixtures/key-data.js';
import { buildDomains, buildSmartContracts } from '../fixtures/domain-data.js';
import { buildRegistries, buildRegistryEntries } from '../fixtures/registry-data.js';
import {
  buildPrivacyGroups,
  buildPrivacyGroupMessages,
  buildPrivacyGroupListeners,
} from '../fixtures/privacy-group-data.js';
import { buildSchemas, buildStates } from '../fixtures/state-data.js';

const rootDir = dirname(fileURLToPath(import.meta.url));
const dataDir = join(rootDir, '../store/data');
mkdirSync(dataDir, { recursive: true });

const transactions = buildTransactions();
const receipts = buildReceipts(transactions);
const events = buildEvents(transactions);
const paladinTransactions = buildPaladinTransactions();
const keys = buildKeys();
const domains = buildDomains();
const smartContracts = buildSmartContracts();
const registries = buildRegistries();
const registryEntries = buildRegistryEntries();
const privacyGroups = buildPrivacyGroups();
const privacyGroupMessages = buildPrivacyGroupMessages(privacyGroups);
const privacyGroupListeners = buildPrivacyGroupListeners(privacyGroups);
const schemas = buildSchemas();
const states = buildStates();

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
writeFileSync(
  join(dataDir, 'keys.json'),
  `${JSON.stringify(keys, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'domains.json'),
  `${JSON.stringify(domains, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'smart-contracts.json'),
  `${JSON.stringify(smartContracts, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'registries.json'),
  `${JSON.stringify(registries, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'registry-entries.json'),
  `${JSON.stringify(registryEntries, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'privacy-groups.json'),
  `${JSON.stringify(privacyGroups, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'privacy-group-messages.json'),
  `${JSON.stringify(privacyGroupMessages, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'privacy-group-listeners.json'),
  `${JSON.stringify(privacyGroupListeners, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'schemas.json'),
  `${JSON.stringify(schemas, null, 2)}\n`
);
writeFileSync(
  join(dataDir, 'states.json'),
  `${JSON.stringify(states, null, 2)}\n`
);

for (const collection of EMPTY_COLLECTIONS) {
  writeFileSync(join(dataDir, `${collection}.json`), '[]\n');
}

const pendingCount = paladinTransactions.filter((tx) => tx.success === undefined).length;
const failedCount = paladinTransactions.filter((tx) => tx.success === false).length;
const folderCount = keys.filter((key) => !key.isKey).length;
const keyCount = keys.filter((key) => key.isKey).length;
const notoCount = smartContracts.filter((c) => c.domainName === 'noto').length;
const zetoCount = smartContracts.filter((c) => c.domainName === 'zeto').length;
const penteCount = smartContracts.filter((c) => c.domainName === 'pente').length;
const inactiveCount = registryEntries.filter((e) => e.active === false).length;
const startedListeners = privacyGroupListeners.filter((l) => l.started).length;
const notoCoinStates = states.filter((s) => s.schema === schemas[0].id).length;

console.log(
  `Generated ${transactions.length} indexed transactions, ${receipts.length} receipts, ${events.length} events`
);
console.log(
  `Generated ${paladinTransactions.length} paladin transactions (${pendingCount} pending, ${failedCount} failed)`
);
console.log(
  `Generated ${keys.length} key entries (${folderCount} folders, ${keyCount} keys)`
);
console.log(
  `Generated ${domains.length} domains, ${smartContracts.length} smart contracts (${notoCount} noto, ${zetoCount} zeto, ${penteCount} pente)`
);
console.log(
  `Generated ${registries.length} registries, ${registryEntries.length} registry entries (${inactiveCount} inactive)`
);
console.log(
  `Generated ${privacyGroups.length} privacy groups, ${privacyGroupMessages.length} messages, ${privacyGroupListeners.length} listeners (${startedListeners} started)`
);
console.log(
  `Generated ${schemas.length} schemas, ${states.length} states (${notoCoinStates} NotoCoin)`
);
