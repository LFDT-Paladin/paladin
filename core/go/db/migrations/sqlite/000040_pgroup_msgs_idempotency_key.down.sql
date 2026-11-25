DROP INDEX pgroup_msgs_idempotency_key;
ALTER TABLE pgroup_msgs DROP COLUMN "idempotency_key";
