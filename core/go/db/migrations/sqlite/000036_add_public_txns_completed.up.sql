ALTER TABLE public_txns ADD "completed" BOOLEAN DEFAULT FALSE;
UPDATE public_txns SET "completed" = TRUE
  WHERE pub_txn_id IN (SELECT pub_txn_id FROM public_completions);
CREATE INDEX public_txns_incomplete ON public_txns ("dispatcher", "from")
  WHERE "completed" IS FALSE;
