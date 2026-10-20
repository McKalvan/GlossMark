-- @gloss represents frt:Consignment conf=0.91
CREATE TABLE consignments (
    id                  UUID         PRIMARY KEY,
    carrier_code        VARCHAR(16)  NOT NULL REFERENCES carriers (code),
    manifest_date       DATE         NOT NULL,
    manifest_reference  VARCHAR(40)  NOT NULL UNIQUE,
    status              VARCHAR(16)  NOT NULL CHECK (status IN ('OPEN', 'CLOSED')),
    closed_at           TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_consignments_open_per_carrier_day
    ON consignments (carrier_code, manifest_date) WHERE status = 'OPEN';

ALTER TABLE shipments
    ADD CONSTRAINT fk_shipments_consignment FOREIGN KEY (consignment_id) REFERENCES consignments (id);
CREATE INDEX idx_shipments_consignment ON shipments (consignment_id);
