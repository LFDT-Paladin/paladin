BEGIN;

DROP INDEX reliable_msgs_node;
CREATE INDEX reliable_msgs_node_sequence ON reliable_msgs ("node", "sequence");

COMMIT;

