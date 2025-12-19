CREATE TABLE sequencer_activities (
  "id"                        BIGINT          NOT NULL,
  "remote_id"                 TEXT            NOT NULL,
  "timestamp"                 BIGINT          NOT NULL,
  "transaction_id"            UUID            NOT NULL,
  "activity_type"             TEXT            NOT NULL,
  "submitting_node"           TEXT            NOT NULL,
  PRIMARY KEY ("id")
);
