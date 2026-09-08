BEGIN;

ALTER TABLE public_txns DROP COLUMN "dispatcher";
ALTER TABLE chained_private_txns DROP COLUMN "id";

COMMIT;
