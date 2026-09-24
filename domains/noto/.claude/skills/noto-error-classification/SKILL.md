---
name: noto-error-classification
description: Decides whether a Noto assemble or endorse failure is a revert or an internal error, per stage, from the provenance of the rejected data, and whether an existing failure category may be reused at a new site. Use before adding, moving or changing any error return, validation check or helper on the ValidateParams, Assemble or Endorse paths in domains/noto/internal/noto.
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

- `errors.go` declares three interfaces. `AssembleError` has `IsAssembleRevert()`, `EndorseError` has `IsEndorseRevert()`, and `AssembleOrEndorseError` embeds both. A function's return type states which stages reach it: `AssembleError` or `EndorseError` for one stage, `AssembleOrEndorseError` for both. The compiler requires every category the function returns to answer for each stage its return type names. It does not check the reverse: a category answering for both stages compiles on a single-stage path, where its extra answer is inert, so that direction is a review decision.
- Init, Prepare, call and receipt paths have no revert outcome. They return plain `error` and discard any category they receive. Many of them share helpers with assemble and endorse; a shared helper classifies for its revert-capable stages only. Never widen a return type because a plain-`error` path calls it.
- A category must be the outermost value. Categories have no `Unwrap`, so wrapping one in `fmt.Errorf`, `i18n.WrapError` or another category discards its answer, and `errors.Is` does not see through one. The entry points are type-safe, so the hazard is a plain-`error` helper that returns a category as `error` and a caller that wraps that result in a different category. Before wrapping a plain-`error` helper's result, check its body does not already return one.
- Each category is a value type `struct{ error }` with constant predicate methods, built in place at the failure site: `return invalidParams{err}`. There are no constructors, no `Unwrap`, and no nil guard. There is no catch-all internal category: a failure that is nobody's fault still names its cause (a failed callback, a stored state that will not parse, a value that will not encode, an assembled state that fails this node's own checks), so the reuse test below has something to check.
- Each category's doc comment states every predicate it implements, its return value and the provenance reason. That repetition is deliberate so the reason is visible on IDE hover at every use; do not prune it.
- The entry points in `noto.go` call `assembleRevertOrError` and `endorseRevertOrError`, which read the predicate and produce either a REVERT response or a Go error. Nothing else in production code reads the predicates; tests go through the helpers below.
- `errors_test.go` has one helper per stage and outcome (`assertAssembleRevert`, `assertAssembleInternal`, `assertEndorseRevert`, `assertEndorseInternal`) and three combined helpers for `AssembleOrEndorseError` (`assertRevert`, `assertEndorseOnlyRevert`, `assertInternal`). Each takes the interface of the function under test, so a test can only assert the stages that function is reachable from.

## Procedure for every new or changed error return

1. Work out which stages can reach the site. The enclosing function's return type says so; a plain `error` return means the failure is classified, if at all, by whichever caller wraps it. If you are changing a classified return type, you are changing which stages every failure in the function must answer for, and every category it returns must implement the new stage.
2. For each stage that reaches the site, say who supplied the data the check rejects, using the table above. Write the answer down before looking at the existing categories.
3. Pick the category by its answers, never by its wording:
   - Reuse an existing category only if, for every stage the site is reached from, its return value matches your answer and the reason in its doc comment holds at your site. A category that also implements a stage the site cannot be reached from is fine; that predicate is inert there, and the compiler only checks the other direction.
   - If a category describes the failure well but its reason does not hold at your site, or its answer differs for any reachable stage, do not reuse it. Add a new category in `errors.go` with a doc comment in the same form: description, then one `IsXRevert <value>: <reason>` paragraph per predicate.
   - A failure that is nobody's fault is still a category with a specific reason. If none of the internal categories' reasons fit, the data was probably not this node's, and the failure is a revert. Wrapping an untrusted-data failure in an internal category is the silent failure mode: it compiles and retries until the transaction times out.
4. When a helper gains a new caller from a stage it did not previously serve, widen its return type to the interface covering both stages and re-answer the provenance question for every category it returns. The compiler will then flag every single-stage category; each one is a decision, not a mechanical fix.
5. Add a test for the new path that calls the helper directly and asserts every stage its return type covers, using the helpers above. These tests are the only guard that a category's answer is right for the data at that site, rather than merely compiling.
6. When you move or refactor a check, confirm the category it returns is still correct for the provenance at the new site.

## Reporting

The final summary must list every error site you added or changed, with its category, the stages that reach it, and a one-line provenance justification per stage, marked for the developer to confirm. Never present a classification as settled; the developer owns the answer.
