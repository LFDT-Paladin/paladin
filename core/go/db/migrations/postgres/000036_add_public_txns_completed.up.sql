BEGIN;

ALTER TABLE public_txns ADD "completed" BOOLEAN DEFAULT FALSE;

-- Backfill: a txn is complete iff a completion row already exists
UPDATE public_txns SET "completed" = TRUE
  WHERE pub_txn_id IN (SELECT pub_txn_id FROM public_completions);

ALTER TABLE public_txns ALTER COLUMN "completed" SET NOT NULL;

-- Partial index over only the outstanding transactions the poll cares about.
-- Predicate is intentionally just "completed IS FALSE" (lean + reusable); the
-- poll's "suspended IS FALSE" is a cheap residual filter on the tiny result.
CREATE INDEX public_txns_incomplete ON public_txns ("dispatcher", "from")
  WHERE "completed" IS FALSE;

COMMIT;
