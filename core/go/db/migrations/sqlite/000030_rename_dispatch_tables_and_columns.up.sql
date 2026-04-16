ALTER TABLE chained_private_txns RENAME TO chained_dispatches;
ALTER TABLE dispatches RENAME COLUMN private_transaction_id TO transaction_id;

DROP INDEX IF EXISTS dispatches_public_private;
ALTER TABLE dispatches DROP COLUMN public_transaction_address;

CREATE UNIQUE INDEX dispatches_public_private ON dispatches("public_transaction_id","transaction_id");
