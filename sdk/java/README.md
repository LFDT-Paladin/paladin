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

## Waiting for transaction receipts

`TxBuilder.send()` returns a `SentTransaction` immediately. Calling `waitForReceipt()`
queries immediately, then waits 100, 200, 400, and 800 milliseconds between unsuccessful
queries before settling at one second. This reduces detection delay for fast transactions
while limiting the extra queries for long-running transactions. Setting
`pollingInterval(Duration)` explicitly selects a fixed interval instead.

The receipt timeout starts after submission returns the transaction ID and also applies
while a receipt RPC is pending. Cancelling the wait stops further polling. It does not
cancel submission or the transaction on the node, and an already-issued receipt RPC may
still finish. Each call to `waitForReceipt()` has an independent timeout and cancellation.

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

The build enforces formatting, Javadoc completeness, and test coverage. All run as part of
`build`/`check`, so CI fails if any is violated.

### Formatting (Spotless + Google Java Format)

```bash
./gradlew :sdk:java:core:spotlessCheck   # verify formatting (runs in build)
./gradlew :sdk:java:core:spotlessApply   # auto-format your changes
```

Enforces Google Java Format (2-space), import ordering, and the Apache-2.0 license header.

### Javadoc (doclint)

```bash
./gradlew :sdk:java:core:javadoc         # lint Javadoc (runs in build/check)
```

Runs the Javadoc tool with `-Xdoclint:all -Werror` over the public API, so the build fails on
any missing or malformed documentation — e.g. an undocumented public method, or a missing
`@param`/`@return`/`@throws` tag. Document every public class, method, and field you add.
Modules that have no documentable types yet (only a `package-info.java`) are skipped until they
gain their first class.

Output: `core/build/docs/javadoc/index.html`.

### Test coverage (JaCoCo)

```bash
./gradlew :sdk:java:core:test            # runs tests + generates the report
./gradlew :sdk:java:core:jacocoTestCoverageVerification   # fails if below threshold
```

Report: `core/build/reports/jacoco/test/html/index.html`.
The build fails if instruction coverage drops below the configured minimum (currently 78%).
