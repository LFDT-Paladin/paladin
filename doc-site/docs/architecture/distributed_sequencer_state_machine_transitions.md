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
    Elect --> Prepared : HeartbeatReceived
    Elect --> Active : HeartbeatReceived
    Elect --> Closing_Flush : HandoverRequest [HasUnconfirmedDispatchedTransactions]
    Elect --> Closing : HandoverRequest [!HasUnconfirmedDispatchedTransactions]
    Elect --> Observing : EndorsementRequestReceived [!HasTransactionsInflight]
    Elect --> Closing_Flush : EndorsementRequestReceived [HasTransactionsInflight && HasUnconfirmedDispatchedTransactions]
    Elect --> Closing : EndorsementRequestReceived [HasTransactionsInflight && !HasUnconfirmedDispatchedTransactions]
    Prepared --> Active : HeartbeatInterval [InactiveGracePeriodExceeded]
    Prepared --> Active : HeartbeatReceived
    Prepared --> Observing : HeartbeatReceived [!HasTransactionsInflight]
    Prepared --> Closing_Flush : HeartbeatReceived [HasTransactionsInflight && HasUnconfirmedDispatchedTransactions]
    Prepared --> Closing : HeartbeatReceived [HasTransactionsInflight && !HasUnconfirmedDispatchedTransactions]
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
    Closing --> Idle : HeartbeatInterval [!HasTransactionsInflight && ClosingGracePeriodExpired && InactiveGracePeriodExceeded]
    Closing --> Observing : HeartbeatInterval [!HasTransactionsInflight && ClosingGracePeriodExpired && !InactiveGracePeriodExceeded]
    Closing --> Elect : TransactionsDelegated [IsHigherPriorityThanCurrentActive && !InactiveGracePeriodExceeded]
    Closing --> Active : TransactionsDelegated [!IsHigherPriorityThanCurrentActive && InactiveGracePeriodExceeded]
```

### Transition Events

| Event | Description |
| --- | --- |
| **CoordinatorCreated** | |
| **EndorsementRequestReceived** | |
| **EpochBoundaryReached** | |
| **HandoverRequest** | |
| **HeartbeatInterval** | |
| **HeartbeatReceived** | |
| **StateTimeoutInterval** | |
| **TransactionStateTransition** | |
| **TransactionsDelegated** | |

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
    Assembling --> Signing : AssembleSuccess [!SignRequirementsFulfilled]
    Assembling --> Endorsement_Gathering : AssembleSuccess [!AttestationPlanFulfilled]
    Assembling --> Confirming_Dispatchable : AssembleSuccess [AttestationPlanFulfilled && !HasDependenciesNotReady]
    Assembling --> Blocked : AssembleSuccess [AttestationPlanFulfilled && HasDependenciesNotReady]
    Assembling --> Pooled : StateTimeoutInterval
    Assembling --> Pooled : AssembleCancelled
    Assembling --> Reverted : AssembleRevert
    Assembling --> Pooled : AssembleError [CanRetryErroredAssemble]
    Assembling --> Evicted : AssembleError [!CanRetryErroredAssemble]
    Assembling --> Pooled : AssembleRequestRejected
    Assembling --> Evicted : AssembleRequestRejected
    Assembling --> Final : AssembleRequestRejected
    Assembling --> PreAssembly_Blocked : DependencyReset
    Assembling --> PreAssembly_Blocked : DependencyConfirmedReverted
    Assembling --> Reverted : ChainedDependencyFailed
    Assembling --> Evicted : ChainedDependencyEvicted
    Signing --> Endorsement_Gathering : Signed [SignRequirementsFulfilled && !AttestationPlanFulfilled]
    Signing --> Confirming_Dispatchable : Signed [SignRequirementsFulfilled && AttestationPlanFulfilled && !HasDependenciesNotReady]
    Signing --> Blocked : Signed [SignRequirementsFulfilled && AttestationPlanFulfilled && HasDependenciesNotReady]
    Signing --> Pooled : SignError [CanRetryErroredAssemble]
    Signing --> Evicted : SignError [!CanRetryErroredAssemble]
    Signing --> Pooled : StateTimeoutInterval
    Endorsement_Gathering --> Confirming_Dispatchable : Endorsed [AttestationPlanFulfilled && !HasDependenciesNotReady]
    Endorsement_Gathering --> Blocked : Endorsed [AttestationPlanFulfilled && HasDependenciesNotReady]
    Endorsement_Gathering --> Pooled : EndorseRevert [EndorseFailureExceedsTolerance]
    Endorsement_Gathering --> Pooled : EndorseError [EndorseFailureExceedsTolerance]
    Endorsement_Gathering --> Pooled : EndorseRequestRejected [EndorseFailureExceedsTolerance]
    Endorsement_Gathering --> Pooled : StateTimeoutInterval
    Blocked --> Confirming_Dispatchable : DependencyReady [!HasDependenciesNotReady]
    Confirming_Dispatchable --> Ready_For_Dispatch : DispatchRequestApproved
    Confirming_Dispatchable --> Pooled : DispatchRequestRejected
    Confirming_Dispatchable --> Evicted : PreDispatchRequestRejected
    Confirming_Dispatchable --> Final : PreDispatchRequestRejected
    Confirming_Dispatchable --> Pooled : StateTimeoutInterval
    Ready_For_Dispatch --> Dispatched : Dispatched
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
| **AssembleCancelled** | |
| **AssembleError** | |
| **AssembleRequestRejected** | |
| **AssembleRevert** | |
| **AssembleSuccess** | |
| **ChainedDependencyEvicted** | |
| **ChainedDependencyFailed** | |
| **ConfirmedReverted** | |
| **ConfirmedSuccess** | |
| **Delegated** | |
| **DependencyConfirmedReverted** | |
| **DependencyReady** | |
| **DependencyReset** | |
| **DependencySelectedForAssemble** | |
| **DispatchRequestApproved** | |
| **DispatchRequestRejected** | |
| **Dispatched** | |
| **EndorseError** | |
| **EndorseRequestRejected** | |
| **EndorseRevert** | |
| **Endorsed** | |
| **HeartbeatInterval** | |
| **PreAssembleDependencyTerminated** | |
| **PreDispatchRequestRejected** | |
| **Selected** | |
| **SignError** | |
| **Signed** | |
| **StateTimeoutInterval** | |

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
| **HeartbeatInterval** | |
| **HeartbeatReceived** | |
| **OriginatorCreated** | |
| **TransactionCreated** | |
| **TransactionStateTransition** | |

---

## Originator Transaction State Machine

```mermaid
%%{init: {'themeVariables': {'background': 'transparent'}}}%%
stateDiagram-v2
    direction LR
    state "Endorsement Gathering" as Endorsement_Gathering
    [*] --> Initial
    Initial --> Confirmed : ConfirmedSuccess
    Initial --> Resolving : Created [HasRequiredVerifiers]
    Initial --> Pending : Created
    Resolving --> Confirmed : ConfirmedSuccess
    Resolving --> Confirmed : ConfirmedReverted
    Resolving --> Pending : VerifiersResolved
    Pending --> Confirmed : ConfirmedSuccess
    Pending --> Confirmed : ConfirmedReverted
    Pending --> Delegated : Delegated
    Delegated --> Confirmed : ConfirmedSuccess
    Delegated --> Confirmed : ConfirmedReverted
    Delegated --> Assembling : AssembleRequestReceived
    Delegated --> Dispatched : Dispatched
    Assembling --> Confirmed : ConfirmedSuccess
    Assembling --> Confirmed : ConfirmedReverted
    Assembling --> Delegated : Delegated
    Assembling --> Signing : AssembleSuccess [HasLocalSignRequirement]
    Assembling --> Endorsement_Gathering : AssembleSuccess
    Assembling --> Reverted : AssembleRevert
    Assembling --> Parked : AssemblePark
    Assembling --> Delegated : AssembleError
    Signing --> Confirmed : ConfirmedSuccess
    Signing --> Confirmed : ConfirmedReverted
    Signing --> Delegated : Delegated
    Signing --> Endorsement_Gathering : SignSuccess
    Signing --> Delegated : SignError
    Signing --> Assembling : AssembleRequestReceived
    Endorsement_Gathering --> Confirmed : ConfirmedSuccess
    Endorsement_Gathering --> Confirmed : ConfirmedReverted
    Endorsement_Gathering --> Delegated : Delegated
    Endorsement_Gathering --> Assembling : AssembleRequestReceived
    Endorsement_Gathering --> Prepared : PreDispatchRequestReceived
    Prepared --> Confirmed : ConfirmedSuccess
    Prepared --> Confirmed : ConfirmedReverted
    Prepared --> Delegated : Delegated
    Prepared --> Dispatched : Dispatched
    Prepared --> Assembling : AssembleRequestReceived
    Dispatched --> Confirmed : ConfirmedSuccess
    Dispatched --> Delegated : ConfirmedReverted
    Dispatched --> Confirmed : ConfirmedReverted
    Dispatched --> Delegated : Delegated
    Dispatched --> Sequenced : NonceAssigned
    Dispatched --> Submitted : Submitted
    Dispatched --> Assembling : AssembleRequestReceived
    Sequenced --> Confirmed : ConfirmedSuccess
    Sequenced --> Delegated : ConfirmedReverted
    Sequenced --> Confirmed : ConfirmedReverted
    Sequenced --> Delegated : Delegated
    Sequenced --> Submitted : Submitted
    Sequenced --> Assembling : AssembleRequestReceived
    Submitted --> Confirmed : ConfirmedSuccess
    Submitted --> Delegated : ConfirmedReverted
    Submitted --> Confirmed : ConfirmedReverted
    Submitted --> Delegated : Delegated
    Submitted --> Assembling : AssembleRequestReceived
    Parked --> Confirmed : ConfirmedSuccess
    Parked --> Confirmed : ConfirmedReverted
    Parked --> Delegated : Delegated
    Parked --> Pending : Resumed
    Confirmed --> Final : Finalize
    Reverted --> Final : Finalize
    Final --> [*]
```

### Transition Events

| Event | Description |
| --- | --- |
| **AssembleError** | |
| **AssemblePark** | |
| **AssembleRequestReceived** | |
| **AssembleRevert** | |
| **AssembleSuccess** | |
| **ConfirmedReverted** | |
| **ConfirmedSuccess** | |
| **Created** | |
| **Delegated** | |
| **Dispatched** | |
| **Finalize** | |
| **NonceAssigned** | |
| **PreDispatchRequestReceived** | |
| **Resumed** | |
| **SignError** | |
| **SignSuccess** | |
| **Submitted** | |
| **VerifiersResolved** | |
