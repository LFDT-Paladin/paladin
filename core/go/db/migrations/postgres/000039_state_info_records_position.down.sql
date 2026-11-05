BEGIN;

ALTER TABLE state_info_records DROP COLUMN "position";

COMMIT;
