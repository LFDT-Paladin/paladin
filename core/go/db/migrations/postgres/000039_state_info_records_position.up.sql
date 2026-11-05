BEGIN;

-- Existing rows have no position, so the order of their transaction's info states is unknown, and the
-- domain is told so when it is asked about that transaction. Every row written from now on has one.
ALTER TABLE state_info_records ADD COLUMN "position" INT;

COMMIT;
