BEGIN;

ALTER TABLE private_smart_contracts ADD COLUMN "indexed_config_bytes" TEXT;
UPDATE private_smart_contracts SET "indexed_config_bytes" = "config_bytes";
ALTER TABLE private_smart_contracts ALTER COLUMN "indexed_config_bytes" SET NOT NULL;

-- Existing rows get an empty event location, which sorts before any real event location. This migration
-- is part of the same release that adds config upgrades, so every existing row still holds the config it
-- was registered with, and any upgrade event is newer.
ALTER TABLE private_smart_contracts ADD COLUMN "config_location" TEXT;
UPDATE private_smart_contracts SET "config_location" = '';
ALTER TABLE private_smart_contracts ALTER COLUMN "config_location" SET NOT NULL;

COMMIT;
