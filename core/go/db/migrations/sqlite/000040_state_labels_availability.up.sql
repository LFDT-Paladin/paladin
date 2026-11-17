-- Denormalize the state's identity and availability columns onto its label rows, so the
-- "available with a label predicate" query can seek a single partial index rather than
-- joining state_labels to states and checking availability per row.
ALTER TABLE state_labels ADD COLUMN "schema"           TEXT;
ALTER TABLE state_labels ADD COLUMN "contract_address" TEXT;
ALTER TABLE state_labels ADD COLUMN "created"          BIGINT;
ALTER TABLE state_labels ADD COLUMN "confirmed"        BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE state_labels ADD COLUMN "spent"            BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE state_int64_labels ADD COLUMN "schema"           TEXT;
ALTER TABLE state_int64_labels ADD COLUMN "contract_address" TEXT;
ALTER TABLE state_int64_labels ADD COLUMN "created"          BIGINT;
ALTER TABLE state_int64_labels ADD COLUMN "confirmed"        BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE state_int64_labels ADD COLUMN "spent"            BOOLEAN NOT NULL DEFAULT FALSE;

-- One-time backfill from the states rows the label rows belong to.
UPDATE state_labels SET
       "schema"           = s."schema",
       "contract_address" = s."contract_address",
       "created"          = s."created",
       "confirmed"        = s."confirmed",
       "spent"            = s."spent"
  FROM states s
 WHERE s."domain_name" = state_labels."domain_name" AND s."id" = state_labels."state";

UPDATE state_int64_labels SET
       "schema"           = s."schema",
       "contract_address" = s."contract_address",
       "created"          = s."created",
       "confirmed"        = s."confirmed",
       "spent"            = s."spent"
  FROM states s
 WHERE s."domain_name" = state_int64_labels."domain_name" AND s."id" = state_int64_labels."state";

-- The access path for available-state queries with an equality label predicate sorted by created:
-- seek the exact prefix (domain, schema, contract, label, value), and the trailing created column
-- returns rows in the requested order so the limit stops the scan without a sort. Spent states are
-- absent (partial predicate); other label values are outside the range. A row enters on confirm
-- and leaves on spend, so the index stays proportional to usage, like states_available.
CREATE INDEX state_labels_available
  ON state_labels ("domain_name", "schema", "contract_address", "label", "value", "created")
  WHERE "confirmed" AND NOT "spent";

CREATE INDEX state_int64_labels_available
  ON state_int64_labels ("domain_name", "schema", "contract_address", "label", "value", "created")
  WHERE "confirmed" AND NOT "spent";
