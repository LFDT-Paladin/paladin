-- Denormalize spent-ness and confirmed-ness onto states so the "available" query can
-- seek a partial index instead of anti-joining the ever-growing state_spend_records.
ALTER TABLE states ADD COLUMN "spent"     BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE states ADD COLUMN "confirmed" BOOLEAN NOT NULL DEFAULT FALSE;

-- One-time backfill from the authoritative record tables.
UPDATE states SET "confirmed" = TRUE
 WHERE ("domain_name", "id") IN (SELECT "domain_name", "state" FROM state_confirm_records);

UPDATE states SET "spent" = TRUE
 WHERE ("domain_name", "id") IN (SELECT "domain_name", "state" FROM state_spend_records);

-- The access path the "available" query seeks: only available rows, created-ordered,
-- scoped the same way as states_by_domain. A row enters on confirm and leaves on spend.
CREATE INDEX states_available
  ON states ("domain_name", "schema", "contract_address", "created")
  WHERE "confirmed" AND NOT "spent";
