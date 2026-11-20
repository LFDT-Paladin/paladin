DROP INDEX IF EXISTS state_labels_available;
DROP INDEX IF EXISTS state_int64_labels_available;

ALTER TABLE state_labels DROP COLUMN "schema";
ALTER TABLE state_labels DROP COLUMN "contract_address";
ALTER TABLE state_labels DROP COLUMN "created";
ALTER TABLE state_labels DROP COLUMN "confirmed";
ALTER TABLE state_labels DROP COLUMN "spent";

ALTER TABLE state_int64_labels DROP COLUMN "schema";
ALTER TABLE state_int64_labels DROP COLUMN "contract_address";
ALTER TABLE state_int64_labels DROP COLUMN "created";
ALTER TABLE state_int64_labels DROP COLUMN "confirmed";
ALTER TABLE state_int64_labels DROP COLUMN "spent";
