DROP TABLE IF EXISTS transactions;
CREATE TABLE transactions (
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
CREATE INDEX transactions_created ON transactions("created", "submit_mode");
CREATE INDEX transactions_domain ON transactions("domain");
CREATE UNIQUE INDEX transactions_idempotency_key ON transactions("idempotency_key");
