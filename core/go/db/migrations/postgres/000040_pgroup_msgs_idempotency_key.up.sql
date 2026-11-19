BEGIN;

ALTER TABLE pgroup_msgs ADD COLUMN "idempotency_key" TEXT;
CREATE UNIQUE INDEX pgroup_msgs_idempotency_key ON pgroup_msgs ("idempotency_key");

COMMIT;
