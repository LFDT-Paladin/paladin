# Paladin

Paladin is a privacy preserving transaction manager for Ethereum.

## Architecture overview

![Paladin Architecture Overview](../images/paladin_runtime.svg)

- Paladin is a sidecar process that runs alongside a Hyperledger Besu node
    - Learn more about [Cloud Native Runtime](../concepts/cloud_native.md)
- Paladin provides **secure channels** of communication to other Paladins over which it can selectively disclose private data
    - Learn more about [Private Data Transports & Endpoint Registry](./data_and_registry.md)
- Paladin supports Privacy Preserving Smart Contracts, and provides samples out-of-the-box
    - Each smart contract has a part of that runs as a EVM smart contract on an **unmodified** EVM blockchain, and a part that runs as part of Paladin
      - Learn more about [ledger layers](./ledger_layers.md)
    - _Some_ implement `tokens backed by Zero-knowledge Proofs`
      - Learn more about [Zeto](./zeto.md)
    - _Some_ implement `tokens backed by Notary Certificates` (issuer/signatory endorsed tokens)
      - Learn more about [Noto](./noto.md)
    - _Some_ implement domains of `EVM Private Smart Contracts` running in `Privacy Groups` 
      - Learn more about [Pente](./pente.md)
      > Provides function similar to that provided by the [Tessera](https://github.com/consensys/tessera) project (successor to [Constellation](https://github.com/Consensys/constellation)) with additional interoperability and other enhancements
    - _All_ use the EVM base ledger as the source of truth for order and finality of transactions
    - _All_ are `atomically interoperable` via the base EVM ledger
      - Learn more about [Atomic interop of privacy preserving smart contracts](./atomic_interop.md)
    - _All_ store state in the EVM base ledger in a securely masked format preserving
        - **Confidentiality**: the data is protected via cryptography, and selectively disclosed on a need to know basis
        - **Anonymity**: the parties involved in a transaction, or set of transactions, cannot be determined without access to the confidential data
        - Learn more about [Full Enterprise Privacy Stack](../concepts/full_stack.md)
- Paladin provides a high performance transaction manager that coordinates transaction assembly, submission and confirmation across Paladin runtimes
    - To any EVM smart contract directly on the **base EVM ledger**
    - To EVM Private Smart Contracts in **privacy groups**, backed by privacy preserving smart contracts
    - To Privacy Preserving Smart Contracts that use **UTXO models** for highly scalable private tokens
    - For atomic swaps between privacy preserving smart contracts
    - Learn more about [Distributed transaction management](./transaction_manager.md) 
- Paladin provides enterprise grade key management integration
    - Managing many keys/identities within a single Paladin runtime
    - With modular integration of remote HSM-backed key management systems
    - Supporting native Ethereum, EIP-712, and ZKP compatible cryptography
    - Learn more about [Signing and Key Management](./key_management.md) 
- Paladin provides a development, configuration, and deployment pipeline for privacy preserving smart contracts
    - Definition of the smart contract functions, inputs and events
    - Supporting EVM programmable private smart contracts and UTXO based token models
    - A set of gRPC code plug points for private lifecycle coordination and state management
        - Programming wallet functions - coin/state selection
        - Programming endorsement coordination / signature collection / sequencer selection
        - Programming transaction verification & proof generation
    - Support for `Java` and `WebAssembly` high performance code modules
    - Learn more about the [Paladin programming model](./programming_model.md)

