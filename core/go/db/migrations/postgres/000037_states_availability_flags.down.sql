BEGIN;

DROP INDEX IF EXISTS states_available;
ALTER TABLE states DROP COLUMN "confirmed";
ALTER TABLE states DROP COLUMN "spent";

COMMIT;
