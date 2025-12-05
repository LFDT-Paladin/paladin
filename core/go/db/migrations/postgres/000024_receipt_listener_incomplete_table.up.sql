BEGIN;

CREATE TABLE receipt_listener_incomplete (
    "listener"           TEXT    NOT NULL,
    "sequence"           BIGINT  NOT NULL,
    "domain_name"        TEXT    NOT NULL,
    "state"              TEXT    NOT NULL,
    PRIMARY KEY ("listener", "sequence"),
    FOREIGN KEY ("listener") REFERENCES receipt_listeners ("name") ON DELETE CASCADE
);

COMMIT;
