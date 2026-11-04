# Zeto variants

## Introduction

Each Zeto token is deployed at a variant. The variant identifies the range of [zeto-contracts](https://github.com/LFDT-Paladin/zeto) releases the token's contracts, circuits and proving keys come from. It is recorded on chain when the token is deployed, and every Paladin node uses it to decide how to build transactions for the token, how to read its events and which proving artifacts to use.

| Variant | zeto-contracts |
|---|---|
| `V0` | 0.2.x (0.2.2) |
| `V1` | 0.5.x (0.5.1) |

One Zeto domain can deploy and transact tokens at several variants. This page describes how to make a variant available on a domain.

## Network-wide prerequisite

Every node in the network must run a Paladin release that supports a variant before any token is deployed at that variant. A node whose release does not support a token's variant treats the token as invalid, and does not process the token's events. Upgrading that node later does not recover the events it skipped.

A node running a release that supports a variant, but without configuration for it, indexes tokens at that variant normally. It cannot submit transactions for those tokens until the variant is configured.

## Proving artifacts

Each variant needs the circuits (`<circuit>_js/<circuit>.wasm`) and proving keys (`<circuit>.zkey`) from its zeto-contracts release. Circuit names are reused across releases with different contents, so each variant's artifacts are kept in their own directory, named after the variant, under the directories configured in `snarkProver`:

```
/app/domains/zeto/zkp/V0/   circuits and proving keys from zeto-contracts 0.2.2
/app/domains/zeto/zkp/V1/   circuits and proving keys from zeto-contracts 0.5.1
```

The Paladin image ships both directories under `/app/domains/zeto/zkp`.

Artifacts are not checked when the node starts. A missing directory or file fails the transaction that needs it, with an error naming the circuit and the variant.

## Deploying and registering the contracts

A Zeto domain has a single factory contract, the domain's `registryAddress`. Tokens are clones of implementation contracts registered on the factory by name.

For each token type you want at a variant:

1. Deploy the Groth16 verifier contracts and the token implementation contract from the variant's zeto-contracts release.
2. Register the implementation on the factory with `registerImplementation(name, implementation)`, where `implementation` holds the implementation address and its verifier addresses.

The name an implementation is registered under must be unique on the factory. When the factory holds the same token type at more than one variant, register each one under a different name, for example `Zeto_AnonNullifier` for `V0` and `Zeto_AnonNullifier_V1` for `V1`.

## Domain configuration

List each variant's implementations under `variants`, and optionally set `defaultVariant`:

```json
{
  "variants": {
    "V0": {
      "implementations": [
        {
          "name": "Zeto_AnonNullifier",
          "circuits": {
            "deposit": { "name": "deposit" },
            "withdraw": { "name": "withdraw_nullifier", "usesNullifiers": true },
            "transfer": { "name": "anon_nullifier_transfer", "usesNullifiers": true },
            "transferLocked": { "name": "anon_nullifier_transferLocked", "usesNullifiers": true }
          }
        }
      ]
    },
    "V1": {
      "implementations": [
        {
          "name": "Zeto_AnonNullifier",
          "registeredName": "Zeto_AnonNullifier_V1",
          "circuits": {
            "deposit": { "name": "deposit" },
            "withdraw": { "name": "withdraw_nullifier", "usesNullifiers": true },
            "transfer": { "name": "anon_nullifier_transfer", "usesNullifiers": true },
            "transferLocked": { "name": "anon" }
          }
        }
      ]
    }
  },
  "defaultVariant": "V1",
  "snarkProver": {
    "circuitsDir": "/app/domains/zeto/zkp",
    "provingKeysDir": "/app/domains/zeto/zkp"
  }
}
```

| Field | Description |
|---|---|
| `variants` | Implementations available at each variant, keyed by variant (`V0`, `V1`). |
| `implementations[].name` | The token type, which is the `tokenName` passed when deploying a token. Unique within a variant. |
| `implementations[].registeredName` | The name the implementation is registered under on the factory. Defaults to `name`. Must be unique across all variants. |
| `implementations[].circuits` | The circuits the token type uses, from the variant's zeto-contracts release. |
| `defaultVariant` | The variant a token is deployed at when the deploy does not name one. Optional; with a single variant configured, that variant is the default. When there is no default, every deploy must name a variant. |
| `snarkProver.circuitsDir`, `snarkProver.provingKeysDir` | Directories holding one subdirectory per variant. |

Non-fungible token types (`Zeto_NfAnon`, `Zeto_NfAnonNullifier`) are only available at `V0`.

### Configuration from earlier releases

Before v1.1, the Zeto domain was configured with `domainContracts`. From v1.1 that format is still accepted, for backwards compatibility. It serves `V0` only, and its artifacts are read directly from the `snarkProver` directories, with no `V0/` subdirectory. With the Paladin image, that means pointing the directories at `V0/`:

```json
{
  "domainContracts": {
    "implementations": [
      { "name": "Zeto_AnonNullifier", "circuits": { ... } }
    ]
  },
  "snarkProver": {
    "circuitsDir": "/app/domains/zeto/zkp/V0",
    "provingKeysDir": "/app/domains/zeto/zkp/V0"
  }
}
```

To add a variant to such a domain, move `domainContracts.implementations` to `variants.V0.implementations`, point the `snarkProver` directories at the directory holding `V0/`, and add the new variant alongside it. A configuration cannot set both `domainContracts` and `variants`.

## Deploying a token

A deploy that only passes `tokenName` deploys at `defaultVariant`:

```json
{
  "type": "private",
  "domain": "zeto",
  "function": "",
  "from": "issuer@node1",
  "abi": [{ "type": "constructor", "inputs": [{ "name": "tokenName", "type": "string" }] }],
  "data": { "tokenName": "Zeto_AnonNullifier" }
}
```

To deploy at a different variant, add a `variant` input to the constructor:

```json
{
  "type": "private",
  "domain": "zeto",
  "function": "",
  "from": "issuer@node1",
  "abi": [{ "type": "constructor", "inputs": [{ "name": "tokenName", "type": "string" }, { "name": "variant", "type": "uint256" }] }],
  "data": { "tokenName": "Zeto_AnonNullifier", "variant": 0 }
}
```

The variant must be configured on the node submitting the deploy.
