BEGIN;

-- Keep existing UNIQUE ("transaction") index from migration 000015_transaction_receipt_listeners but add index on sequence
CREATE INDEX transaction_receipts_transaction_sequence ON transaction_receipts ("transaction", "sequence" DESC);

COMMIT;
