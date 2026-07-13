# Paladin UI

### Introduction
This is a light-weight compact UI used for development/debugging purposes.

### Setup
1. Install dependencies
```
cd client
npm i
```
2. Run UI against Paladin node 1
```
npm run dev:node1
```
3. Run UI against Paladin node 2
```
npm run dev:node2
```
4. Run UI against Paladin node 3
```
npm run dev:node3
```

### Using the UI
Open a web browser and navigate to `http://localhost:3000`, `http://localhost:3002` and `http://localhost:3003` for nodes 1, 2 and 3 respectively.

### Testing (E2E)

End-to-end tests use Playwright with a mock JSON-RPC server so no live Paladin node is required.

1. Install dependencies and Playwright browsers:
```
cd e2e
npm i
npx playwright install chromium
```

2. Run all tests (starts the mock RPC server and Vite in test mode automatically):
```
npm test
```

3. Run a single suite:
```
npm test -- tests/transactions.spec.ts
```

4. Interactive UI mode:
```
npm run test:ui
```

5. Run query engine unit tests:
```
npm run test:query
```

#### Mock server fixtures

E2E tests use a file-backed mock JSON-RPC server. List data lives in JSON files under `e2e/mock-server/store/data/`. RPC methods are mapped in `e2e/mock-server/methods.json` and query filtering/sorting is handled by the shared query engine in `e2e/mock-server/query/`.

To regenerate fixture data (indexed transactions, receipts, events, Paladin submissions, keys, domains, registries, privacy groups, states, and transports):

```
cd e2e
npm run generate-fixtures
```

This writes:
- `store/data/indexed-transactions.json`, `transaction-receipts.json`, `indexed-events.json` — Transactions page
- `store/data/paladin-transactions.json` — Submissions page (25 pending / 5 failed)
- `store/data/keys.json` — Keys page (folders + keys for list and explorer views)
- `store/data/domains.json`, `smart-contracts.json` — Domains page (noto / pente / zeto + contracts)
- `store/data/registries.json`, `registry-entries.json` — Registries page (default registry + entries)
- `store/data/privacy-groups.json`, `privacy-group-messages.json`, `privacy-group-listeners.json` — Privacy Groups pages
- `store/data/schemas.json`, `states.json` — States page (schemas + private states)
- `store/data/transport-peers.json`, `transport-messages.json` — Transports connections and reliable messages

You can also edit the JSON files directly to change mock responses without changing server code.
