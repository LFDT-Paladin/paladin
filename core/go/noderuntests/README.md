# Core component tests

## Run the whole test suite using gradle

Run tests with gas free chain:

```
gradle core:go:componentTestSQLite
```

Run tests with chain that uses gas:

```
gradle core:go:componentTestWithGasSQLite
```

# Core coordination tests

## Run the whole test suite using gradle

```
gradle core:go:coordinationTestPostgres
```

# Run in VS code

To run individual tests with the `Go` VS Code extension

1. Start the test infrastructure

```
gradle startTestInfra
```

Choose which variant of the tests you would like to run, using `.vscode/settings.json`

To debug the tests in zero gas price mode - or "free gas" mode:

```js
    "go.testTags": "testdbpostgres,besu_free_gas",
    "go.buildTags": "testdbpostgres,besu_free_gas"
```

To debug the tests in non-zero gas price mode - or "paid gas" mode:

```js
    "go.testTags": "testdbpostgres,besu_paid_gas",
    "go.buildTags": "testdbpostgres,besu_paid_gas"
```

## Unit tests

There are also coordination unit tests which are alongside the code. The tests in `/noderuntests` are not unit tests but in fact launch all of the node components and run full nodes, and hence have been kept here alongside component tests for clarity.
