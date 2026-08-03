BEGIN;

CREATE TABLE pending_private_state_data (
    "contract"     VARCHAR NOT NULL,
    "state_id"     VARCHAR NOT NULL,
    "block_number" BIGINT  NOT NULL,
    PRIMARY KEY ("state_id")
);
CREATE INDEX ppsd_contract_block ON pending_private_state_data ("contract", "block_number");

COMMIT;
