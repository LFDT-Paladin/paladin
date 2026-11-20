CREATE TABLE private_smart_contracts_new (
    "deploy_tx"            UUID    NOT NULL,
    "domain_address"       TEXT    NOT NULL,
    "address"              TEXT    NOT NULL,
    "config_bytes"         TEXT    NOT NULL,
    "created"              BIGINT  DEFAULT 0,
    "indexed_config_bytes" TEXT    NOT NULL,
    "config_location"      TEXT    NOT NULL,
    PRIMARY KEY ("address")
);

-- Existing rows get an empty event location, which sorts before any real event location. This migration
-- is part of the same release that adds config upgrades, so every existing row still holds the config it
-- was registered with, and any upgrade event is newer.
INSERT INTO private_smart_contracts_new
    SELECT "deploy_tx", "domain_address", "address", "config_bytes", "created", "config_bytes", ''
    FROM private_smart_contracts;
DROP TABLE private_smart_contracts;
ALTER TABLE private_smart_contracts_new RENAME TO private_smart_contracts;
CREATE INDEX private_smart_contracts_domain_address ON private_smart_contracts("domain_address");

-- Index cannot be unique or it's an attack vector to block indexing by deploying a different contract with same deploy TX
CREATE INDEX private_smart_contracts_deploy_tx ON private_smart_contracts("deploy_tx");
