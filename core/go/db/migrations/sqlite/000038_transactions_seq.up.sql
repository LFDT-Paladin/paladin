-- when transactions are insterted in batches, created is not unique. An incrementing
-- seq column ensures that we can order within a batch, which is necessary for correct
-- submission ordering of chained transactions.
--
-- SQLite cannot add an auto-incrementing column with ALTER TABLE: AUTOINCREMENT is only available
-- on an INTEGER PRIMARY KEY. So the table is rebuilt, with "id" moving from PRIMARY KEY to a
-- UNIQUE index to make room.
DROP TABLE IF EXISTS transactions;
CREATE TABLE transactions (
  "seq"                       INTEGER         PRIMARY KEY AUTOINCREMENT,
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
  FOREIGN KEY ("abi_ref") REFERENCES abis ("hash") ON DELETE CASCADE
);
CREATE UNIQUE INDEX transactions_id ON transactions("id");
CREATE INDEX transactions_created ON transactions("created", "submit_mode");
CREATE INDEX transactions_domain ON transactions("domain");
CREATE UNIQUE INDEX transactions_idempotency_key ON transactions("idempotency_key");
