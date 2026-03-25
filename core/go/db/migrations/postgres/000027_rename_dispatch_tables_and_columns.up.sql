BEGIN;

ALTER TABLE chained_private_txns RENAME TO chained_dispatches;
ALTER TABLE dispatches RENAME COLUMN private_transaction_id TO transaction_id;

COMMIT;
