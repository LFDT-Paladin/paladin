# How transaction sequencing information is exchanged between nodes

Paladin nodes communicate transaction data between themselves in order to assemble, endorse, and confirm Paladin transactions.

To aid with problem diagnostics and to allow an identity who submits a transaction to track its progress through sequencing and on to the base ledger, additional information is passed between certain nodes. This includes:

 - Sequencing activity
    - This aids with problem diagnosis. For example a submitting node is able to confirm if any attempts have been made to progress the transaction
    - Currently the only sequencing activity that is recorded is `dispatch` activity. This is recorded when a node intends to submit a public transaction to the base ledger (but may not necessarily have done so yet), or when a node creates a chained transaction (see #chain_transactions) 
 - Public transaction submissions to the base ledger
    - This also aids with problem diagnosis. For example a submitting node is able to confirm if any attempts have been made to submit the base ledger transaction, and whether those attempts have been successful.

The following diagram gives an example of the data flows between nodes for a private transaction submitted byte **member1** to **Node 1**.

 - Node 1 is the node the transaction is submitted to
 - Node 2 is Notary for this `Noto` domain so it is the node that coordinates all `Noto` transactions
 - Nodes 3 and 4 also participate in this `Noto` domain but are not involved in this particular Paladin transaction in a privacy group with Node 2. 

![Data flows](diagrams/paladin-node-data-flows.svg){.zoomable-image}

 - Queries to Node 1 return all transaction data, as well as the public transaction submissions and any sequencing records.
 - Queries to Node 2 return the receipt for the Paladin transaction, as well as the public transaction submissions and any sequencing records. The node does not store or return the Paladin transaction itself.
 - Queries to nodes 3 and 4 do not return any information about the Paladin transaction or the public transaction submissions. The nodes have not received private or public transaction data.

## Chained transactions

Some Paladin transactions are assembled into a new private transaction, called a "chained" transaction. A typical example of this is where a `Noto` token is deployed with a hook to delegate transaction approval to a private smart contract deployed to a `Pente` privacy group.

The following diagram shows the flow of data between the nodes involved in both the original and the chained transaction:

![Chained transaction flows](diagrams/paladin-node-data-flows-chained-transactions.svg){.zoomable-image}


1. TX1 - in this case a Noto transfer - is submitted to Node 1
2. The notary for the token is Node 2, so TX1 is coordinated by Node 2
3. The notary assembles TX1 into a chained transaction, TX2
4. TX2 is coordinated by any of the members of the privacy group. In this case Node 3 is acting as the coordinator
5. Node 3 assembles TX2 into a public base ledger transaction
6. Node 2 receives the following information
   - A copy of every public transaction submission from Node 3 to the base ledger
   - A record of sequencing activities at Node 3
      - Currently the only sequencing activity recorded is `dispatch` which indicate that a private or public transaction has been submitted for processing.
      - The `dispatch` activity for TX2 includes a `remote_id` which is the `dispatch_id` of the public base ledger transaction submitted by Node 3
      - This allows an administrator of Node 2 to provide an administrator of Node 3 with an identifier to aid with problem diagnosis and tracking.
7. Node 1 receives the following information 
   - A receipt for TX1 when Node 2 determines that TX2 has been confirmed
   - A record of the sequencing activities at Node 2
      - Currently the only sequencing activity recorded is `dispatch` 
      - The `dispatch` activity for TX1 includes a `remote_id` which is the `dispatch_id` of TX2
      - This allows an administrator of Node 1 to provide an administrator of Node 2 with an identifier to aid with problem diagnosis and tracking
      - Node 1 does not receive any information about the nature of TX2, including who the participants of the privacy group are or who submitted the public transaction to the base ledger