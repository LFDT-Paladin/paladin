---
name: noto-error-classification
description: Decides whether a Noto assemble or endorse failure is a revert or an internal error, per phase, from the provenance of the rejected data. Use before adding, moving or changing any error return, validation check or helper on the ValidateParams, Assemble or Endorse paths in domains/noto/internal/noto.
---

# Noto error classification

## The two outcomes

Assemble and endorse each have two outcomes for a failure. A **revert** says the transaction is invalid and must never be submitted: the platform fails it immediately and reports the reason to the application. An **internal error** says something unexpected went wrong here and now: the platform retries.

Both wrong answers are expensive. An invalid transaction classified as an internal error retries until it times out and the application never learns why. An unexpected failure classified as a revert terminally fails a transaction that would have succeeded on the next attempt.

## Provenance decides

The right answer depends on who supplied the data the check rejected, and that differs by phase.

| Phase | Trusted (this node produced it) | Untrusted (came from outside) |
|---|---|---|
| Assemble, on the originator | Transaction specification, resolved verifiers, selected states, contract config, anything read from the local state store or a callback | Function parameters and the operation they request |
| Endorse, on an endorser | The endorser's own state store reads and callbacks | Everything in the request: specification, verifiers, states, signatures, config, IDs |

A rejected trusted value is an internal error (a bug or a transient fault). A rejected untrusted value is a revert. Anything assemble can reject is still invalid at endorse, so a failure is never a revert at assemble only.

## How the code expresses it

- Classification is keyed on the Noto message an error carries. `classify.go` holds one table, `reverts`, mapping each message that reverts to a pair of explicit booleans, one per phase. An error reverts in a phase only if its message has a row and that phase's boolean is true. No row reverts at assemble without also reverting at endorse.
- Everything else is internal and retries, and nothing checks that this was a decision rather than an omission. That covers unlisted Noto messages and errors from other components that reach the paths unwrapped: pldtypes, the toolkit, the signer, the JSON and ABI encoders, and core failing a callback, which the toolkit surfaces as an unkeyed string. None of these can ever have a row. If such a failure is the transaction's fault, the site must wrap it in a Noto message whose row matches. At a callback site the provenance question is about what went into the request, not about core.
- Paladin's i18n errors do not unwrap, so only the outermost key counts. Wrapping a keyed cause in another Noto message reclassifies it as that message.
- `classify_test.go` has one helper per phase and outcome: `assertRevertAssemble`, `assertRevertEndorse`, `assertInternalAssemble`, `assertInternalEndorse`. A test states both phases with two calls. There is no exhaustiveness test; the per-helper tests are the only guard.

## Procedure for every new or changed error return

1. Work out which phases can reach the site. A helper shared by assemble and endorse is reached by both.
2. For each phase, say who supplied the data the check rejects, using the table above.
3. Pick the message:
   - The error must carry a Noto message whose row matches your two answers exactly. A bare library error or a foreign-keyed one is internal; if it should revert, wrap it with `i18n.WrapError` and a Noto message.
   - If an existing message says the right thing but its row differs from your answers, do not reuse it. Mint a new message and place it. The stored-state messages exist for exactly this reason: the same parse failure is a revert for a state in the transaction and internal for a state read from this node's own store.
   - A new message that should revert needs a row with both booleans written out. Nothing fails if you forget; the error silently retries instead.
4. Never pick a message for its wording alone. A message reused where the provenance differs silently changes retry behaviour, and nothing in the compiler will catch it.
5. Add a test for the new path that calls the helper directly and asserts both phases, one `assertRevert*` or `assertInternal*` call each. The per-failure helper tests are the only durable guard on classification, and they are what catches a row that only happens to be right for today's callers.
6. When you move or refactor a check, confirm the error it returns still carries the message you expect at the new site, and that no caller wraps it in a differently classified message.

## Reporting

The final summary must list every error site you added or changed, with its classification and a one-line provenance justification, marked for the developer to confirm. Never present a classification as settled; the developer owns the answer.
