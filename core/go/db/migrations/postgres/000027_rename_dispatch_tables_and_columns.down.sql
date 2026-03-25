BEGIN;

ALTER TABLE dispatches RENAME COLUMN transaction_id TO private_transaction_id;
ALTER TABLE chained_dispatches RENAME TO chained_private_txns;

COMMIT;
