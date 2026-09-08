CREATE TABLE pending_private_state_data (
    "contract"     TEXT    NOT NULL,
    "state_id"     TEXT    NOT NULL,
    "block_number" INTEGER NOT NULL,
    PRIMARY KEY ("state_id")
);
CREATE INDEX ppsd_contract_block ON pending_private_state_data ("contract", "block_number");
