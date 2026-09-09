DROP INDEX IF EXISTS dispatches_public_private;
ALTER TABLE dispatches ADD COLUMN public_transaction_address TEXT NOT NULL DEFAULT '';
ALTER TABLE dispatches RENAME COLUMN transaction_id TO private_transaction_id;
CREATE UNIQUE INDEX dispatches_public_private ON dispatches("public_transaction_address","public_transaction_id","private_transaction_id");

ALTER TABLE chained_dispatches RENAME TO chained_private_txns;
