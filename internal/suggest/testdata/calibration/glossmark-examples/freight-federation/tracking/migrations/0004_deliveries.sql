-- @gloss represents .ProofOfDelivery conf=0.85
CREATE TABLE deliveries (
    id               UUID PRIMARY KEY,
    shipment_id      UUID        NOT NULL UNIQUE REFERENCES tracked_shipments (shipment_id),
    tracking_number  TEXT        NOT NULL,
    delivered_at     TIMESTAMPTZ NOT NULL,
    lat              DOUBLE PRECISION NOT NULL,
    lng              DOUBLE PRECISION NOT NULL,
    proof_kind       TEXT        NOT NULL CHECK (proof_kind IN ('SIGNATURE', 'PHOTO', 'GEOFENCE')),
    proof_ref        TEXT        NOT NULL,
    promised_start   TIMESTAMPTZ NOT NULL,
    within_window    BOOLEAN     NOT NULL,
    published_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX deliveries_unpublished_idx ON deliveries (created_at) WHERE published_at IS NULL;
CREATE INDEX deliveries_delivered_at_idx ON deliveries (delivered_at);
