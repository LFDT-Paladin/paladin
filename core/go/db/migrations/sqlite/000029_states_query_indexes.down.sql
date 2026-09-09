DROP INDEX state_labels_domain_label_value;
DROP INDEX state_int64_labels_domain_label_value;

DROP INDEX states_by_domain;
CREATE INDEX states_by_domain ON states ("domain_name", "schema", "contract_address");
