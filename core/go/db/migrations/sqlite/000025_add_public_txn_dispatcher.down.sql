BEGIN;

ALTER TABLE public_txn DROP COLUMN "dispatcher";

COMMIT;
