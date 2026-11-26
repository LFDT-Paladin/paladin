BEGIN;

DROP INDEX IF EXISTS state_labels_available;
DROP INDEX IF EXISTS state_int64_labels_available;

ALTER TABLE state_labels
  DROP COLUMN "schema",
  DROP COLUMN "contract_address",
  DROP COLUMN "created",
  DROP COLUMN "confirmed",
  DROP COLUMN "spent";

ALTER TABLE state_int64_labels
  DROP COLUMN "schema",
  DROP COLUMN "contract_address",
  DROP COLUMN "created",
  DROP COLUMN "confirmed",
  DROP COLUMN "spent";

COMMIT;
