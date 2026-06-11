# Paladin Java SDK

Native Java SDK for [Paladin](https://github.com/LFDT-Paladin/paladin) — enables enterprise
JVM applications to interact with a Paladin node without going through the Go or TypeScript SDKs.

Work in progress...

## Modules

| Module             | Description                                      |
|--------------------|--------------------------------------------------|
| `core`             | Data models and primitives                       |
| `client`           | HTTP + WebSocket transport, RPC client           |
| `domains`          | Helpers for Noto, Pente, and Zeto protocols      |
| `testing`          | Mock client, WireMock stubs, Testcontainers      |
| `integration-test` | End-to-end tests against a real Paladin node     |

## Building

```bash
./gradlew :sdk:java:core:build
./gradlew :sdk:java:client:build
./gradlew :sdk:java:domains:build
./gradlew :sdk:java:testing:build
./gradlew :sdk:java:integration-test:build
```

Requires JDK 17+.
