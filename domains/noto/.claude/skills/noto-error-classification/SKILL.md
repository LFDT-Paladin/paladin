---
name: noto-error-classification
description: Decides whether a Noto assemble or endorse failure is a revert or an internal error, per stage, from the provenance of the rejected data, and which error type a site must build. Use before adding, moving or changing any error return, validation check or helper on the ValidateParams, Assemble or Endorse paths in domains/noto/internal/noto.
---

# Noto error classification

## The two outcomes

Assemble and endorse each have two outcomes for a failure. A **revert** says the transaction is invalid and must never be submitted: the platform fails it immediately and reports the reason to the application. An **internal error** says something unexpected went wrong here and now: the platform retries.

Both wrong answers are expensive. An invalid transaction classified as an internal error retries until it times out and the application never learns why. An unexpected failure classified as a revert terminally fails a transaction that would have succeeded on the next attempt.

## Provenance decides

The right answer depends on who supplied the data the check rejected, and that differs by stage.

| Stage | Trusted (this node produced it) | Untrusted (came from outside) |
|---|---|---|
| Assemble, on the originator | The specification's serialisation, contract config and address, resolved verifiers, selected states, state and transaction IDs, anything read from the local state store or a callback | The function the caller chose and the parameters they supplied, even though they arrive inside the specification |
| Endorse, on an endorser | The endorser's own state store reads and callbacks | Everything in the request: specification, verifiers, states, signatures, config, IDs |

A rejected trusted value is an internal error (a bug or a transient fault). A rejected untrusted value is a revert. The same check on the same field can therefore be internal at assemble and a revert at endorse. When a check compares an untrusted value with a trusted one, the untrusted value is what is rejected. A lookup keyed on a caller-supplied value that finds nothing is a revert even though a later retry might find it, because available-state queries exclude states held by in-flight transactions and the caller still asked for something that is not there.

ValidateParams runs before every stage over the same specification. Its return type is `AssembleOrEndorseError`, and its failures are the caller's or the originator's function choice and parameters, so a revert at both.

## How the code expresses it

- `errors.go` declares three interfaces. `AssembleError` has `IsAssembleRevert()`, `EndorseError` has `IsEndorseRevert()`, and `AssembleOrEndorseError` embeds both. A function's return type states which stages reach it: `AssembleError` or `EndorseError` for one stage, `AssembleOrEndorseError` for both. The compiler requires every value the function returns to answer for each stage its return type names. It does not check the reverse: a value answering for both stages compiles on a single-stage path, where its extra answer is inert, so that direction is a review decision.
- Each interface has exactly one implementation, and a site builds it as a literal with the answer for each stage written out: `assembleError{err, false}`, `endorseError{err, true}`, `assembleOrEndorseError{err, false, true}`. The field order is the error, then the assemble answer, then the endorse answer. There are no constructors, no `Unwrap`, no catch-all, and no nil guard: a literal is built only inside the branch that has detected the failure, so the error it wraps is never nil. A tail-position `return x, classify(err)` is not possible; write the `if err != nil` branch.
- A single-stage path builds the type of its own stage, never `assembleOrEndorseError` with an inert answer. That keeps the return type honest: if the function later gains a caller from the other stage, widening its return type makes every literal in it fail to compile, and each one is then re-answered.
- The reason for each answer lives in a comment at the site when it is not obvious from the check itself. There is no shared vocabulary of categories to lean on, so a `false` next to a rejected value that came from outside, or a `true` next to a value this node built, has to be explained or it is a bug.
- Init, Prepare, call and receipt paths have no revert outcome. They return plain `error` and discard any classification they receive. Many of them share helpers with assemble and endorse; a shared helper classifies for its revert-capable stages only. Never widen a return type because a plain-`error` path calls it.
- A classified value must be the outermost value. The types have no `Unwrap`, so wrapping one in `fmt.Errorf`, `i18n.WrapError` or another literal discards its answer, and `errors.Is` does not see through one. The entry points are type-safe, so the hazard is a plain-`error` helper that returns a classified value as `error` and a caller that wraps that result in a new literal. Before wrapping a plain-`error` helper's result, check its body does not already return one.
- The entry points in `noto.go` call `assembleRevertOrError` and `endorseRevertOrError`, which read the predicate and produce either a REVERT response or a Go error. Nothing else in production code reads the predicates; tests go through the helpers below.
- `errors_test.go` has one helper per stage and outcome (`assertAssembleRevert`, `assertAssembleInternal`, `assertEndorseRevert`, `assertEndorseInternal`) and two combined helpers for `AssembleOrEndorseError` (`assertRevert`, `assertEndorseOnlyRevert`). Each takes the interface of the function under test, so a test can only assert the stages that function is reachable from.

## Procedure for every new or changed error return

1. Work out which stages can reach the site. The enclosing function's return type says so; a plain `error` return means the failure is classified, if at all, by whichever caller wraps it. If you are changing a classified return type, you are changing which stages every failure in the function must answer for, and every literal it returns must change type.
2. For each stage that reaches the site, say who supplied the data the check rejects, using the table above. Write the answer down before writing the literal.
3. Build the literal of the enclosing function's return type with those answers. Do not copy the literal from a neighbouring site: the neighbour's data may have a different owner. `true` for a rejected untrusted value, `false` for a rejected trusted value, per stage.
4. If the answer at any stage is not evident from the check itself, add a one-line comment at the site saying whose data was rejected. An internal answer for a value that arrived in the request, or a revert for a value this node built, always needs one.
5. When a helper gains a new caller from a stage it did not previously serve, widen its return type to the interface covering both stages and re-answer the provenance question at every literal in it. The compiler flags each one; each is a decision, not a mechanical fix.
6. Add a test for the new path that calls the helper directly and asserts every stage its return type covers, using the helpers above. These tests are the only guard that an answer is right for the data at that site, rather than merely compiling.
7. When you move or refactor a check, confirm the answers in its literal are still correct for the provenance at the new site.

## Reporting

The final summary must list every error site you added or changed, with the literal it builds, the stages that reach it, and a one-line provenance justification per stage, marked for the developer to confirm. Never present a classification as settled; the developer owns the answer.
