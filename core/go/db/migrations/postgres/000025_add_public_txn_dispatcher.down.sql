BEGIN;

ALTER TABLE public_txns DROP COLUMN "dispatcher";

COMMIT;
