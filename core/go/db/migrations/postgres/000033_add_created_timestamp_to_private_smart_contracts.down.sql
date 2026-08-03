BEGIN;

ALTER TABLE private_smart_contracts DROP COLUMN "created";

COMMIT;
