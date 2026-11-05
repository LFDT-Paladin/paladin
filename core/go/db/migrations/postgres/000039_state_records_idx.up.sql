BEGIN;

-- The index of each update record within the blockchain finalization
ALTER TABLE state_spend_records ADD COLUMN "idx" INT;
ALTER TABLE state_read_records ADD COLUMN "idx" INT;
ALTER TABLE state_confirm_records ADD COLUMN "idx" INT;
ALTER TABLE state_info_records ADD COLUMN "idx" INT;

COMMIT;
