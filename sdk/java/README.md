# Paladin Java SDK

Native Java SDK for [Paladin](https://github.com/LFDT-Paladin/paladin) — enables enterprise
JVM applications to interact with a Paladin node using a Java native library.

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

Requires JDK 21+.

## Code quality

The build enforces formatting and test coverage. Both run as part of `build`/`check`,
so CI fails if either is violated.

### Formatting (Spotless + Google Java Format)

```bash
./gradlew :sdk:java:core:spotlessCheck   # verify formatting (runs in build)
./gradlew :sdk:java:core:spotlessApply   # auto-format your changes
```

Enforces Google Java Format (2-space), import ordering, and the Apache-2.0 license header.

### Test coverage (JaCoCo)

```bash
./gradlew :sdk:java:core:test            # runs tests + generates the report
./gradlew :sdk:java:core:jacocoTestCoverageVerification   # fails if below threshold
```

Report: `core/build/reports/jacoco/test/html/index.html`.
The build fails if instruction coverage drops below the configured minimum (currently 78%).
