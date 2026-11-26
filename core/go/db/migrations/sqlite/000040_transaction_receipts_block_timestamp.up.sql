-- The timestamp of the base ledger block, for receipts of transactions/events that made it on-chain
ALTER TABLE transaction_receipts ADD COLUMN "block_timestamp" BIGINT;
