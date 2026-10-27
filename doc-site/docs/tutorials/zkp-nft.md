# Non-Fungible Tokens based on Zeto

The code for this tutorial can be found in [examples/zeto-nft](https://github.com/LFDT-Paladin/paladin/blob/main/examples/zeto-nft).

This shows how to use [Zeto](../../architecture/zeto/) non-fungible tokens to represent unique assets, such as artwork or property titles, whose ownership and properties stay private as they pass between parties on different Paladin nodes.

## Running the example

Follow the [Getting Started](../../getting-started/installation/) instructions to set up a Paladin environment, and then follow the example [README](https://github.com/LFDT-Paladin/paladin/blob/main/examples/zeto-nft/README.md) to run the code.

## Scenario: passing a unique asset between three owners

An issuer on node1 mints a single NFT to `alice` on node1. Alice transfers it to `bob` on node2, and bob transfers it on to `carol` on node3.

Below is a walkthrough of each step in the example, with an explanation of what it does.

### Create the NFT token

```typescript
const zetoFactory = new ZetoFactory(paladin1, "zeto");
const artwork = await zetoFactory
  .newZetoNonFungible(issuer, { tokenName: "Zeto_NfAnon" })
  .waitForDeploy();
```

This creates a new instance of the Zeto domain, using the [Zeto_NfAnon](https://github.com/hyperledger-labs/zeto/blob/main/solidity/contracts/zeto_nf_anon.sol) contract. This results in a new cloned contract on the base ledger, with a new unique address.

Minting is restricted to the issuer, the deployer account of the contract.

### Mint an NFT

```typescript
const mintReceipt = await artwork
  .mint(issuer, {
    mints: [{ to: alice.verifier, uri: "https://example.com/artwork/1" }],
  })
  .waitForReceipt();
```

The issuer mints an NFT to `alice`. The Zeto domain assigns the token a random token ID, and records the token as a private state holding its owner, token ID and URI. Only a commitment to that state, a hash that reveals none of its contents, is written to the base ledger.

### Find the NFTs an owner holds

```typescript
const states = await paladin.pstate.queryContractStates(
  "zeto",
  zetoAddress,
  nftSchemaID,
  { limit: 100, eq: [{ field: "owner", value: ownerKey }] },
  "confirmed"
);
```

An NFT has no balance, so the example finds an owner's tokens by querying the unspent states of the token contract that are owned by the owner's Baby Jubjub public key. Only the owner's own node holds the private state, so the query must be sent to that node.

### Transfer the NFT across nodes

```typescript
const receipt = await artwork
  .using(alice.paladin)
  .transfer(alice.verifier, {
    transfers: [{ to: bob.verifier, tokenID }],
  })
  .waitForReceipt();
```

Alice transfers the NFT to `bob` by its token ID. The identity `alice` exists on node1, so the transfer must be sent through that node (`.using(alice.paladin)`). Alice's node generates a zero-knowledge proof that the new state keeps the token ID and URI of the old one, spends the old state on the base ledger and sends the new state privately to bob's node.

The receipt on alice's node does not mean bob's node has received the new state yet, so the example waits until bob's node reports bob as the owner. Bob then transfers the NFT on to `carol` in the same way, through node2, which shows that a node can spend an NFT it received from another node.

## Next Steps

Next, explore Zeto tokens further to understand how compliant **KYC** processes can be integrated with zero-knowledge proof-backed tokens.

[Continue to the Private Stablecoin Tutorial →](./private-stablecoin.md)
