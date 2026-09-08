# Transaction sequencing

Private transactions implemented using UTXO states require Paladin to compile a list of input and output UTXOs which represent the before and after state for the transaction.

Different types of private transaction have different UTXO behaviours.

For the Zeto domain for example, every UTXO belongs exlusively to the current owner and can only be spent by the current owner. This requires only basic transaction sequencing since no other identities can attempt to spend the same state.

Other domains, specifically those that can involve multiple parties potentially spending the same states, require more complex sequencing.

For the Pente domain for example, there may be 2 parties who wish to modify the same EVM storage variable. This is valid, but transaction 2 must "spend" the state that was output when transaction 1 modified the variable if both transactions are to succeed. If they both attempt to spend the same state the second transaction will fail.

The component of Paladin that manages the sequencing of private transactions for all domains is called the **Distributed Sequencer**.

![Distributed Sequencer](diagrams/light/sequencer.svg#only-light){.zoomable-image}
![Distributed Sequencer](diagrams/dark/sequencer.svg#only-dark){.zoomable-image}

The term "distributed" refers to the fact that for any given instance of a domain the sequencing of transactions is distributed among the nodes that are participating in that domain instance. The sequencing of transactions is distributed among the nodes to share the workload and resources across the peers. For some domain types it is important to ensure that only one participating node acts as the sequencer at a given point in time. For other domain types the participating nodes are responsible for their own sequencing.

### Notarized token domains

Transaction ordering must be coordinated by the party acting as the notary because the notary must endorse every transaction before submitting it to the base ledger. This also allows the notary to optimistically coordinate subsequent transactions by tracking newly created states that other parties can attempt to use for their transactions before the state has actually been confirmed.

### Privacy preserving token domains

For domains that use technologies such as zero-knowledge proofs to validate the spending and minting of states, each party coordinates its own transactions because no central party has visibility of the spent and unspent states. Every node is responsible for building transactions in the correct order to prevent attempting to spend states more than once.

### Privacy groups and private smart contracts

For private smart contracts deployed to a privacy group, every change to the state trie made by a private smart contract transaction must visible to subsequent transactions invoking the same private smart contract. This ensures that the new state trie created by the first transaction is used as the starting state for the next transaction. This requires that only one member of the privacy group acts as the transaction coordinator at a given time, but since all members of the privacy group have visibility of the private smart contract state any party can act as the coordinator.

Paladin uses a deterministic algorithm for selecting which node to act as the sequencing node if none is currently doing so. If a node becomes temporarily unavailable the algorithm chooses an alternative node to act as the sequencing node, repeating the process until one of the nodes begins sequencing transactions.

---

### Supporting enhanced domains in the future

While the design of the distributed sequencer provides a robust algorithm for efficiently assembling transactions on one of a number of available nodes, the current reference domain implementations built in to Paladin do not yet exploit all of the functionality the algorithm provides.

It is expected that future enhancements to the reference domains will allow them to exploit the advanced sequencing algorithm described above. For example, notarized tokens could be enhanced to support multiple parties acting as the notary and using some form of consensus algorithm to agree on transaction submission (known as `K` of `N` agreement). This would allow tranasctions to be successfully confirmed on the base ledger even when one notarizing party is unavailable for an extended period of time.
