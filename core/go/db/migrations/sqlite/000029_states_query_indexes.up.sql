CREATE INDEX state_labels_domain_label_value ON state_labels ("domain_name", "label", "value");
CREATE INDEX state_int64_labels_domain_label_value ON state_int64_labels ("domain_name", "label", "value");

DROP INDEX states_by_domain;
CREATE INDEX states_by_domain ON states ("domain_name", "schema", "contract_address", "created");
