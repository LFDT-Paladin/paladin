DROP INDEX CONCURRENTLY reliable_msgs_node;
CREATE INDEX CONCURRENTLY reliable_msgs_node_sequence ON reliable_msgs ("node", "sequence");
