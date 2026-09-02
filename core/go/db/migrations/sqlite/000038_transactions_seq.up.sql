-- when transactions are insterted in batches, created is not unique. An incrementing
-- seq column ensures that we can order within a batch, which is necessary for correct
-- submission ordering of chained transactions.
--
-- SQLite cannot add an auto-incrementing column with ALTER TABLE: AUTOINCREMENT is only available
-- on an INTEGER PRIMARY KEY. So the table is rebuilt following the procedure in
-- https://sqlite.org/lang_altertable.html#otheralter - copy into a new table, drop the old one,
-- rename the new one into its place - with "id" moving from PRIMARY KEY to a UNIQUE index to make
-- room. The UNIQUE index keeps "id" a valid parent key for the four tables whose foreign keys
-- reference transactions("id").
--
-- The copy is ordered by "created" so existing rows get sequences in the closest approximation of
-- write order the old schema can express. That is only approximate for rows sharing a timestamp -
-- exactly the ambiguity this column exists to remove - which is acceptable because it applies only
-- to rows written before the migration. The indexes are recreated after the rename because index
-- names are unique across the schema, so the old table's must be dropped with it first.
--
-- Requires PRAGMA foreign_keys to be off, as the procedure above documents, otherwise dropping the
-- old table cascades deletes into its dependants. Paladin never enables it (see sqlite.go), and it
-- cannot be set here in any case - golang-migrate wraps each migration in a transaction, and SQLite
-- silently ignores the pragma inside one.
CREATE TABLE transactions_new (
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

INSERT INTO transactions_new
  ("id", "idempotency_key", "created", "type", "submit_mode", "abi_ref", "function", "domain", "from", "to", "data")
  SELECT
   "id", "idempotency_key", "created", "type", "submit_mode", "abi_ref", "function", "domain", "from", "to", "data"
  FROM transactions ORDER BY "created";

DROP TABLE transactions;
ALTER TABLE transactions_new RENAME TO transactions;

CREATE UNIQUE INDEX transactions_id ON transactions("id");
CREATE INDEX transactions_created ON transactions("created", "submit_mode");
CREATE INDEX transactions_domain ON transactions("domain");
CREATE UNIQUE INDEX transactions_idempotency_key ON transactions("idempotency_key");
