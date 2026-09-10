The following qualifiers can be used in queries to the state store:

- `unconfirmed` - states where no transaction has yet been processed from the blockchain to confirm the state
- `confirmed` - states that have been confirmed by a blockchain transaction, and not yet been spent in a subsequent transaction. A state is only confirmed for use while it also remains unspent.
- `spent` - states that have been marked spent as a result of indexing a blockchain transaction
- `all` - all states stored in this node, regardless of status