BEGIN;

-- The block at which each state's confirmation was indexed. It is kept on the confirm record, and copied onto the
-- state and its label rows beside its confirmed flag so that an available-state query can be limited to states confirmed before
-- a given block. A confirmation has no block when it was indexed before this migration and its transaction has no receipt,
-- or when it has the zero transaction ID because no transaction created the state. An available-state query always
-- includes a state with no block.
ALTER TABLE state_confirm_records ADD COLUMN "block_number" BIGINT;
ALTER TABLE states ADD COLUMN "confirmed_block" BIGINT;
ALTER TABLE state_labels ADD COLUMN "confirmed_block" BIGINT;
ALTER TABLE state_int64_labels ADD COLUMN "confirmed_block" BIGINT;

-- One-time backfill of confirmations indexed before this migration, from the receipt their transaction completed
-- with. Those with no receipt under their transaction ID keep no block.
UPDATE state_confirm_records r SET "block_number" = tr."block_number"
  FROM transaction_receipts tr
 WHERE tr."transaction" = r."transaction";

UPDATE states s SET "confirmed_block" = c."block_number"
  FROM state_confirm_records c
 WHERE c."domain_name" = s."domain_name" AND c."state" = s."id" AND s."confirmed";

UPDATE state_labels l SET "confirmed_block" = s."confirmed_block"
  FROM states s
 WHERE s."domain_name" = l."domain_name" AND s."id" = l."state" AND l."confirmed";

UPDATE state_int64_labels l SET "confirmed_block" = s."confirmed_block"
  FROM states s
 WHERE s."domain_name" = l."domain_name" AND s."id" = l."state" AND l."confirmed";

COMMIT;
