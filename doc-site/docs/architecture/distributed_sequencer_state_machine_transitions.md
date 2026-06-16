# State machine transition detail

Detailed state diagrams showing every transition event and guard condition for each of the four distributed sequencer state machines.

*Auto-generated from source*

## Coordinator State Machine

```mermaid
%%{init: {'themeVariables': {'background': 'transparent'}}}%%
stateDiagram-v2
    direction LR
    state "Active Flush" as Active_Flush
    state "Closing Flush" as Closing_Flush
    [*] --> Initial
    Initial --> Idle : CoordinatorCreated
    Idle --> Observing : HeartbeatReceived
    Idle --> Observing : EndorsementRequestReceived
    Idle --> Active : TransactionsDelegated
    Observing --> Idle : HeartbeatInterval [InactiveGracePeriodExceeded]
    Observing --> Elect : TransactionsDelegated [IsHigherPriorityThanCurrentActive]
    Elect --> Active : StateTimeoutInterval
    Elect --> Observing : HeartbeatReceived [!HasTransactionsInflight]
    Elect --> Closing_Flush : HeartbeatReceived [HasTransactionsInflight && HasUnconfirmedDispatchedTransactions]
    Elect --> Closing : HeartbeatReceived [HasTransactionsInflight && !HasUnconfirmedDispatchedTransactions]
    Elect --> Closing_Flush : HandoverRequest [HasUnconfirmedDispatchedTransactions]
    Elect --> Closing : HandoverRequest [!HasUnconfirmedDispatchedTransactions]
    Elect --> Observing : EndorsementRequestReceived [!HasTransactionsInflight]
    Elect --> Closing_Flush : EndorsementRequestReceived [HasTransactionsInflight && HasUnconfirmedDispatchedTransactions]
    Elect --> Closing : EndorsementRequestReceived [HasTransactionsInflight && !HasUnconfirmedDispatchedTransactions]
    Prepared --> Active : HeartbeatInterval [InactiveGracePeriodExceeded]
    Prepared --> Active : HeartbeatReceived
    Prepared --> Closing_Flush : HandoverRequest [HasUnconfirmedDispatchedTransactions]
    Prepared --> Closing : HandoverRequest [!HasUnconfirmedDispatchedTransactions]
    Prepared --> Observing : EndorsementRequestReceived [!HasTransactionsInflight]
    Prepared --> Closing_Flush : EndorsementRequestReceived [HasTransactionsInflight && HasUnconfirmedDispatchedTransactions]
    Prepared --> Closing : EndorsementRequestReceived [HasTransactionsInflight && !HasUnconfirmedDispatchedTransactions]
    Active --> Idle : HeartbeatInterval [!HasTransactionsInflight]
    Active --> Closing_Flush : HeartbeatReceived [HasUnconfirmedDispatchedTransactions]
    Active --> Closing : HeartbeatReceived [!HasUnconfirmedDispatchedTransactions]
    Active --> Closing_Flush : HandoverRequest [HasUnconfirmedDispatchedTransactions]
    Active --> Closing : HandoverRequest [!HasUnconfirmedDispatchedTransactions]
    Active --> Closing_Flush : EndorsementRequestReceived [HasUnconfirmedDispatchedTransactions]
    Active --> Closing : EndorsementRequestReceived [!HasUnconfirmedDispatchedTransactions]
    Active --> Active_Flush : EpochBoundaryReached [MustFlushToRotateSigningIdentity]
    Active_Flush --> Closing_Flush : HeartbeatReceived
    Active_Flush --> Closing_Flush : HandoverRequest [HasUnconfirmedDispatchedTransactions]
    Active_Flush --> Closing : HandoverRequest [!HasUnconfirmedDispatchedTransactions]
    Active_Flush --> Closing_Flush : EndorsementRequestReceived
    Active_Flush --> Active : TransactionStateTransition [!HasUnconfirmedDispatchedTransactions]
    Closing_Flush --> Elect : TransactionsDelegated [IsHigherPriorityThanCurrentActive]
    Closing_Flush --> Closing : TransactionStateTransition [!HasUnconfirmedDispatchedTransactions]
    Closing --> Idle : HeartbeatInterval [statemachine.GuardAnd(
								statemachine.GuardNot(guard_HasTransactionsInflight),
								guard_ClosingGracePeriodExpired,
								guard_InactiveGracePeriodExceeded,
							)]
    Closing --> Observing : HeartbeatInterval [statemachine.GuardAnd(
								statemachine.GuardNot(guard_HasTransactionsInflight),
								guard_ClosingGracePeriodExpired,
								statemachine.GuardNot(guard_InactiveGracePeriodExceeded),
							)]
    Closing --> Elect : TransactionsDelegated [statemachine.GuardAnd(
								guard_IsHigherPriorityThanCurrentActive,
								statemachine.GuardNot(guard_InactiveGracePeriodExceeded),
							)]
    Closing --> Active : TransactionsDelegated [statemachine.GuardAnd(
								statemachine.GuardNot(guard_IsHigherPriorityThanCurrentActive),
								guard_InactiveGracePeriodExceeded,
							)]
```

### Transition Events

| Event | Description |
| --- | --- |
| **CoordinatorCreated** |  |
| **TransactionsDelegated** |  |
| **StateTimeoutInterval** |  |
| **HandoverRequest** | pushed by transport_client when a CoordinatorHandoverRequest message is received from a higher-priority node |
| **EndorsementRequestReceived** | pushed by transport_client when an EndorsementRequest message arrives for this coordinator |
| **EpochBoundaryReached** | queued internally by getAndRefreshBlockHeight when the effective block height advances to a new epoch |
| **HeartbeatInterval** | (shared event from sequencer common package) |
| **HeartbeatReceived** | (shared event from sequencer common package) |
| **TransactionStateTransition** | (shared event from sequencer common package) |

---

## Coordinator Transaction State Machine

```mermaid
%%{init: {'themeVariables': {'background': 'transparent'}}}%%
stateDiagram-v2
    direction LR
    state "PreAssembly Blocked" as PreAssembly_Blocked
    state "Endorsement Gathering" as Endorsement_Gathering
    state "Confirming Dispatchable" as Confirming_Dispatchable
    state "Ready For Dispatch" as Ready_For_Dispatch
    [*] --> Initial
    Initial --> Reverted : Delegated [HasRevertedChainedDependency]
    Initial --> Evicted : Delegated [HasEvictedChainedDependency]
    Initial --> PreAssembly_Blocked : Delegated [HasUnassembledDependencies]
    Initial --> Pooled : Delegated [!HasUnassembledDependencies]
    PreAssembly_Blocked --> Pooled : DependencySelectedForAssemble [!HasUnassembledDependencies]
    PreAssembly_Blocked --> Pooled : PreAssembleDependencyTerminated [!HasUnassembledDependencies]
    PreAssembly_Blocked --> Reverted : ChainedDependencyFailed
    PreAssembly_Blocked --> Evicted : ChainedDependencyEvicted
    Pooled --> Assembling : Selected
    Pooled --> PreAssembly_Blocked : DependencyReset
    Pooled --> PreAssembly_Blocked : DependencyConfirmedReverted
    Pooled --> Reverted : ChainedDependencyFailed
    Pooled --> Evicted : ChainedDependencyEvicted
    Assembling --> Endorsement_Gathering : AssembleSuccess [!AttestationPlanFulfilled]
    Assembling --> Confirming_Dispatchable : AssembleSuccess [AttestationPlanFulfilled && !HasDependenciesNotReady]
    Assembling --> Blocked : AssembleSuccess [AttestationPlanFulfilled && HasDependenciesNotReady]
    Assembling --> Pooled : StateTimeoutInterval
    Assembling --> Pooled : AssembleCancelled
    Assembling --> Reverted : AssembleRevert
    Assembling --> Pooled : AssembleError [CanRetryErroredAssemble]
    Assembling --> Evicted : AssembleError [!CanRetryErroredAssemble]
    Assembling --> Pooled : AssembleRequestRejected
    Assembling --> PreAssembly_Blocked : DependencyReset
    Assembling --> PreAssembly_Blocked : DependencyConfirmedReverted
    Assembling --> Reverted : ChainedDependencyFailed
    Assembling --> Evicted : ChainedDependencyEvicted
    Endorsement_Gathering --> Confirming_Dispatchable : Endorsed [AttestationPlanFulfilled && !HasDependenciesNotReady]
    Endorsement_Gathering --> Blocked : Endorsed [AttestationPlanFulfilled && HasDependenciesNotReady]
    Endorsement_Gathering --> Pooled : EndorseRevert [EndorseFailureExceedsTolerance]
    Endorsement_Gathering --> Pooled : EndorseError [EndorseFailureExceedsTolerance]
    Endorsement_Gathering --> Pooled : EndorseRequestRejected [EndorseFailureExceedsTolerance]
    Endorsement_Gathering --> Pooled : StateTimeoutInterval
    Endorsement_Gathering --> PreAssembly_Blocked : DependencyReset
    Endorsement_Gathering --> PreAssembly_Blocked : DependencyConfirmedReverted
    Endorsement_Gathering --> Reverted : ChainedDependencyFailed
    Blocked --> Confirming_Dispatchable : DependencyReady [!HasDependenciesNotReady]
    Blocked --> PreAssembly_Blocked : DependencyReset
    Blocked --> PreAssembly_Blocked : DependencyConfirmedReverted
    Blocked --> Reverted : ChainedDependencyFailed
    Confirming_Dispatchable --> Ready_For_Dispatch : DispatchRequestApproved
    Confirming_Dispatchable --> Pooled : DispatchRequestRejected
    Confirming_Dispatchable --> Evicted : PreDispatchRequestRejected
    Confirming_Dispatchable --> Pooled : StateTimeoutInterval
    Confirming_Dispatchable --> PreAssembly_Blocked : DependencyReset
    Confirming_Dispatchable --> PreAssembly_Blocked : DependencyConfirmedReverted
    Confirming_Dispatchable --> Reverted : ChainedDependencyFailed
    Ready_For_Dispatch --> Dispatched : Dispatched
    Ready_For_Dispatch --> PreAssembly_Blocked : DependencyReset
    Ready_For_Dispatch --> PreAssembly_Blocked : DependencyConfirmedReverted
    Ready_For_Dispatch --> Reverted : ChainedDependencyFailed
    Dispatched --> Confirmed : ConfirmedSuccess
    Dispatched --> PreAssembly_Blocked : ConfirmedReverted [CanRetryRevert && HasUnassembledDependencies]
    Dispatched --> Pooled : ConfirmedReverted [CanRetryRevert && !HasUnassembledDependencies]
    Dispatched --> Reverted : ConfirmedReverted [!CanRetryRevert]
    Dispatched --> Reverted : ChainedDependencyFailed
    Reverted --> Final : HeartbeatInterval [HasFinalizingGracePeriodPassedSinceStateChange]
    Confirmed --> Final : HeartbeatInterval [HasFinalizingGracePeriodPassedSinceStateChange]
    Final --> [*]
    Evicted --> [*]
```

### Transition Events

| Event | Description |
| --- | --- |
| **Delegated** | Transaction initially received by the coordinator.  Might seem redundant explicitly modeling this as an event rather than putting this logic into the constructor, but it is useful to make the initial state transition rules explicit in the state machine definitions |
| **DependencySelectedForAssemble** | the transaction delegated immediately before the transaction from the same originator has been selected for assembly |
| **Selected** | selected from the pool as the next transaction to be assembled |
| **AssembleSuccess** | assembler returned a successful assembly |
| **AssembleRevert** | assembler returned a revert (domain said assembly is invalid) |
| **AssembleError** | assembler returned an unexpected error |
| **AssembleRequestRejected** | originator rejected the assemble request (e.g. block height tolerance exceeded) |
| **AssembleCancelled** | the assemble attempt has been cancelled |
| **Endorsed** | endorsement received from one endorser |
| **EndorseRevert** | endorser responded that the assembly is invalid (domain REVERT) |
| **EndorseError** | endorser encountered an unexpected error processing the request |
| **EndorseRequestRejected** | endorser rejected the request before processing (e.g. block height tolerance) |
| **DependencyReady** | another transaction, for which this transaction has a dependency on, has become ready for dispatch |
| **DependencyReset** | another transaction, for which this transaction has a dependency on, has been reset |
| **DependencyConfirmedReverted** | another transaction, for which this transaction has a dependency on, has been confirmed as reverted |
| **DispatchRequestApproved** | dispatch confirmation received from the originator |
| **DispatchRequestRejected** | dispatch confirmation response received from the originator with a rejection |
| **Dispatched** | dispatched to the public TX manager |
| **ConfirmedSuccess** | confirmation received from the blockchain of a successful transaction |
| **ConfirmedReverted** | confirmation received from the blockchain of a reverted transaction |
| **StateTimeoutInterval** | event emitted when a state has exceeded its maximum allowed duration |
| **PreDispatchRequestRejected** | originator has rejected the pre-dispatch request (NOT_CURRENT_DELEGATE or TRANSACTION_UNKNOWN) |
| **ChainedDependencyFailed** | a chained (same-coordinator) dependency has been permanently finalized as failed |
| **ChainedDependencyEvicted** | a chained (same-coordinator) dependency has been evicted (e.g. assembly failure threshold exceeded) |
| **PreAssembleDependencyTerminated** | the pre-assemble (FIFO ordering) predecessor has reached a terminal state |
| **HeartbeatInterval** | (shared event from sequencer common package) |

---

## Originator State Machine

```mermaid
%%{init: {'themeVariables': {'background': 'transparent'}}}%%
stateDiagram-v2
    direction LR
    [*] --> Initial
    Initial --> Idle : OriginatorCreated
    Idle --> Observing : HeartbeatReceived
    Idle --> Sending : TransactionCreated
    Observing --> Idle : HeartbeatInterval [InactiveGracePeriodExceeded]
    Observing --> Sending : TransactionCreated
    Sending --> Observing : TransactionStateTransition [!HasTransactions]
```

### Transition Events

| Event | Description |
| --- | --- |
| **OriginatorCreated** | fired once by Start to drive the initial coordinator selection |
| **TransactionCreated** | a new transaction has been created and is ready to be sent to the coordinator TODO maybe name something like Intent created? |
| **HeartbeatInterval** | (shared event from sequencer common package) |
| **HeartbeatReceived** | (shared event from sequencer common package) |
| **TransactionStateTransition** | (shared event from sequencer common package) |

---

## Originator Transaction State Machine

```mermaid
%%{init: {'themeVariables': {'background': 'transparent'}}}%%
stateDiagram-v2
    direction LR
    state "Endorsement Gathering" as Endorsement_Gathering
    [*] --> Initial
    Initial --> Confirmed : ConfirmedSuccess
    Initial --> Pending : Created
    Pending --> Confirmed : ConfirmedSuccess
    Pending --> Delegated : Delegated
    Delegated --> Confirmed : ConfirmedSuccess
    Delegated --> Assembling : AssembleRequestReceived
    Delegated --> Dispatched : Dispatched
    Assembling --> Confirmed : ConfirmedSuccess
    Assembling --> Delegated : Delegated
    Assembling --> Endorsement_Gathering : AssembleAndSignSuccess
    Assembling --> Reverted : AssembleRevert
    Assembling --> Parked : AssemblePark
    Assembling --> Delegated : AssembleError
    Endorsement_Gathering --> Confirmed : ConfirmedSuccess
    Endorsement_Gathering --> Delegated : Delegated
    Endorsement_Gathering --> Assembling : AssembleRequestReceived [!AssembleRequestMatchesPreviousResponse]
    Endorsement_Gathering --> Prepared : PreDispatchRequestReceived
    Prepared --> Confirmed : ConfirmedSuccess
    Prepared --> Delegated : Delegated
    Prepared --> Dispatched : Dispatched
    Prepared --> Assembling : AssembleRequestReceived [!AssembleRequestMatchesPreviousResponse]
    Dispatched --> Confirmed : ConfirmedSuccess
    Dispatched --> Delegated : ConfirmedReverted [WillRetry]
    Dispatched --> Confirmed : ConfirmedReverted [!WillRetry]
    Dispatched --> Delegated : Delegated
    Dispatched --> Sequenced : NonceAssigned
    Dispatched --> Submitted : Submitted
    Dispatched --> Assembling : AssembleRequestReceived
    Sequenced --> Confirmed : ConfirmedSuccess
    Sequenced --> Delegated : ConfirmedReverted [WillRetry]
    Sequenced --> Confirmed : ConfirmedReverted [!WillRetry]
    Sequenced --> Delegated : Delegated
    Sequenced --> Submitted : Submitted
    Sequenced --> Assembling : AssembleRequestReceived
    Submitted --> Confirmed : ConfirmedSuccess
    Submitted --> Delegated : ConfirmedReverted [WillRetry]
    Submitted --> Confirmed : ConfirmedReverted [!WillRetry]
    Submitted --> Delegated : Delegated
    Submitted --> Assembling : AssembleRequestReceived
    Parked --> Confirmed : ConfirmedSuccess
    Parked --> Delegated : Delegated
    Parked --> Pending : Resumed
    Confirmed --> Final : Finalize
    Reverted --> Final : Finalize
    Final --> [*]
```

### Transition Events

| Event | Description |
| --- | --- |
| **Created** | Transaction initially received by the originator or has been loaded from the database after a restart / swap-in |
| **ConfirmedSuccess** | confirmation received from the blockchain of base ledge transaction successful completion |
| **ConfirmedReverted** | confirmation received from the blockchain of base ledge transaction failure |
| **Delegated** | transaction has been delegated to a coordinator |
| **AssembleRequestReceived** | coordinator has requested that we assemble the transaction |
| **AssembleAndSignSuccess** | we have successfully assembled the transaction and signing module has signed the assembled transaction |
| **AssembleRevert** | we have failed to assemble the transaction |
| **AssemblePark** | we have parked the transaction |
| **AssembleError** | an unexpected error occurred while trying to assemble the transaction |
| **Dispatched** | coordinator has dispatched the transaction to a public transaction manager |
| **PreDispatchRequestReceived** | coordinator has requested confirmation that the transaction is OK to be dispatched |
| **Resumed** | Received an RPC call to resume a parked transaction |
| **NonceAssigned** | the public transaction manager has assigned a nonce to the transaction |
| **Submitted** | the transaction has been submitted to the blockchain |
| **Finalize** | internal event to trigger transition from terminal states (Confirmed/Reverted) to State_Final for cleanup |
