BEGIN;

DROP INDEX reliable_msgs_node_sequence;
CREATE INDEX reliable_msgs_node ON reliable_msgs ("node");

COMMIT;

