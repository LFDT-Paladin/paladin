CREATE TABLE state_read_records_new (
    "domain_name" TEXT NOT NULL,
    "state"       TEXT NOT NULL,
    "transaction" UUID NOT NULL,
    PRIMARY KEY ("domain_name", "state", "transaction")
);
INSERT INTO state_read_records_new SELECT * FROM state_read_records;
DROP TABLE state_read_records;
ALTER TABLE state_read_records_new RENAME TO state_read_records;
CREATE INDEX state_read_records_transaction ON state_read_records("transaction");

CREATE TABLE state_info_records_new (
    "domain_name" TEXT NOT NULL,
    "state"       TEXT NOT NULL,
    "transaction" UUID NOT NULL,
    PRIMARY KEY ("domain_name", "state", "transaction")
);
INSERT INTO state_info_records_new SELECT * FROM state_info_records;
DROP TABLE state_info_records;
ALTER TABLE state_info_records_new RENAME TO state_info_records;
CREATE INDEX state_info_records_transaction ON state_info_records("transaction");
