BEGIN;

CREATE TABLE sequencer_activity (
  "id"                        UUID            NOT NULL,
  "correlation_id"            TEXT            NOT NULL,
  "timestamp"                 BIGINT          NOT NULL,
  "transaction_id"            UUID            NOT NULL,
  "activity_type"             TEXT            NOT NULL,
  "submitting_node"           TEXT            NOT NULL,
  PRIMARY KEY ("id")
);

COMMIT;
