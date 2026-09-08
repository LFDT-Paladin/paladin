BEGIN;

ALTER TABLE private_smart_contracts ADD COLUMN "created" BIGINT DEFAULT 0;

-- Backfill creation time for existing contracts
UPDATE private_smart_contracts psc
SET "created" = t."created"
FROM transactions t
WHERE t."id" = psc."deploy_tx";

COMMIT;
