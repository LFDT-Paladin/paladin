BEGIN;

ALTER TABLE state_read_records DROP CONSTRAINT state_read_records_pkey;
ALTER TABLE state_read_records ADD PRIMARY KEY ("domain_name", "state");

ALTER TABLE state_info_records DROP CONSTRAINT state_info_records_pkey;
ALTER TABLE state_info_records ADD PRIMARY KEY ("domain_name", "state");

COMMIT;
