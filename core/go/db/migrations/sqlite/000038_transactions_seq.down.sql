-- Reverse of the up migration: rebuild the table without "seq", restoring "id" as the PRIMARY KEY.
-- Same procedure and the same requirement that PRAGMA foreign_keys is off.
--
-- The copy is ordered by "seq" so that rows are written back in the order the dropped column
-- recorded, which is the best that "created" alone can then represent.
CREATE TABLE transactions_old (
  "id"                        UUID            NOT NULL,
  "idempotency_key"           TEXT,
  "created"                   BIGINT          NOT NULL,
  "type"                      TEXT            NOT NULL,
  "submit_mode"               VARCHAR         NOT NULL,
  "abi_ref"                   TEXT            NOT NULL,
  "function"                  TEXT,
  "domain"                    TEXT,
  "from"                      TEXT            NOT NULL,
  "to"                        TEXT,
  "data"                      TEXT,
  PRIMARY KEY ("id"),
  FOREIGN KEY ("abi_ref") REFERENCES abis ("hash") ON DELETE CASCADE
);

INSERT INTO transactions_old
  ("id", "idempotency_key", "created", "type", "submit_mode", "abi_ref", "function", "domain", "from", "to", "data")
  SELECT
   "id", "idempotency_key", "created", "type", "submit_mode", "abi_ref", "function", "domain", "from", "to", "data"
  FROM transactions ORDER BY "seq";

DROP TABLE transactions;
ALTER TABLE transactions_old RENAME TO transactions;

CREATE INDEX transactions_created ON transactions("created", "submit_mode");
CREATE INDEX transactions_domain ON transactions("domain");
CREATE UNIQUE INDEX transactions_idempotency_key ON transactions("idempotency_key");
