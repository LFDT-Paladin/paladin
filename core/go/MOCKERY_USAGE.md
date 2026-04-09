# Mockery Usage (Go Mocks)

We use [mockery](https://github.com/vektra/mockery) to generate Go interface mocks for testing. As of v2.44+, and in preparation for v3, all mock generation is managed via config files using the `packages` feature and templated variables.

## How to Generate Mocks

Mocks are generated using Gradle tasks, which invoke mockery with the appropriate config files. No interface or package arguments should be passed via the CLI—everything is defined in the config files (e.g., `.mockery.yml`, `.mockery.sequencer.yml`, etc.).

To generate all mocks:

```sh
gradle makeMocks
```

This will use the config files and templated variables to generate/update all mocks. You should not see any deprecation warnings about CLI usage.

## Adding New Mocks

1. Add the relevant interface/package to the appropriate `.mockery*.yml` config file under the `packages` section.
2. Run `gradle makeMocks` to generate the new mocks.

## References
- [Mockery v2.44+ Packages Configuration](https://vektra.github.io/mockery/v2.44/features/#packages-configuration)
- [Migrating to Packages](https://vektra.github.io/mockery/v2.44/migrating_to_packages/)

If you see any warnings, ensure you are not passing interface/package info via CLI and that your config files are up to date.
