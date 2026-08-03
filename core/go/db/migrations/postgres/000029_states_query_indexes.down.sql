BEGIN;

DROP INDEX IF EXISTS state_labels_domain_label_value;
DROP INDEX IF EXISTS state_int64_labels_domain_label_value;

DROP INDEX IF EXISTS states_by_domain;
CREATE INDEX states_by_domain ON states ("domain_name", "schema", "contract_address");

COMMIT;
