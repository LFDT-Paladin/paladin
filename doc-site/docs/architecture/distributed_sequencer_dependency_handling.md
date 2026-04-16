# Transaction dependencies

Paladin transactions can produce new states (e.g. minting new tokens), consume existing states (e.g. spending an existing token) or both.

The states created and consumed by a transaction are determined when the originator node assembles the transaction. The originator will create and select states based on its assembly implementation.

It is common for multiple Paladin transactions to be related to each other in terms of their state changes. An example would be that TX1 creates a state by minting a new token and TX2 consumes that state by transfering the token to another identity. Paladin tracks transaction state changes through its use of a `grapher` component. If a transaction creates a state that another transaction consumes, it is said to be a `pre-req` of that transaction. The `grapher` component maintains `pre-req` links between transaction based on the output of the assembly phase and makes certain decisions based on the `grapher`.

There are 2 key aspects of Paladin's behaviour that depend on the grapher's knowledge or transaction relationships.

## Re-assembly when a pre-req transaction reverts

If a transaction TX2 has a dependency on transaction TX1 and Paladin observes that the TX1 base ledger transaction has reverted, Paladin will automatically move TX2 back into the assembly phase of sequencing. Paladin knows that the failure of the TX1 base ledger transaction will result in the failure of TX2. On re-assembly of TX2 it is possible that the dependencies will change such that TX2 is no longer dependent on TX1. For example a subsequent successful transaction TX3 may have completed in the mean time and the originator selects states from TX3 as inputs to TX2. Hence the transaction depedency chains are not always the same after reassembly.

![Re-assembly](diagrams/paladin-transaction-dependency-assembly.svg){.zoomable-image}

## Dispatch control based on signing identities

Some domains or Paladin configurations dispatch base ledger transactions using a new signing key per transaction. This is supported and can be suitable for domains where the verification of the base ledger transaction is not based on the message sender of the public transaction but instead on the EIP-712 signatures in the payload.

For transactions that are dependent on each other, the use of a different signing address per transaction can result in the transctions being included in blocks out of the order they were assembled by Paladin. In this case the public transactions will revert, for example because TX2 is trying to spend state that TX1 creates but TX1 has not yet been included in a base ledger block.

In most cases Paladin will automatically use the dependency chains built into its `grapher` component and determine when it is not safe to dispatch dependent transactions in parallel.

![Dispatch control](diagrams/paladin-transaction-dispatch-control.svg){.zoomable-image}

Paladin provides manual control points to restrict the maximum number of base ledger tranasctions to dispatch at a time per domain instance. See `maxDispatchAhead`. Setting this to a value of `1` will result in only a single Paladin transaction for a given domain instance being dispatched at a time. Until that tranasction has been confirmed or finalized, the next assembled transaction will not be dispatched. In most cases it should not be necessary to modify `maxDispatchAhead`.
