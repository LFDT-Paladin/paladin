ALTER TABLE private_smart_contracts ADD COLUMN "created" BIGINT DEFAULT 0;

-- No backfill on creation time (sqlite not used in deployment)
