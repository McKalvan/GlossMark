-- @gloss represents frt:OnTimeDeliveryRate conf=0.8
CREATE TABLE delivery_outcomes (
    shipment_id    UUID         PRIMARY KEY REFERENCES shipments (id),
    carrier_code   VARCHAR(16)  NOT NULL REFERENCES carriers (code),
    service_level  VARCHAR(16)  NOT NULL,
    window_start   TIMESTAMPTZ  NOT NULL,
    window_end     TIMESTAMPTZ  NOT NULL,
    delivered_at   TIMESTAMPTZ  NOT NULL,
    within_window  BOOLEAN      NOT NULL,
    proof_kind     VARCHAR(16)  CHECK (proof_kind IN ('SIGNATURE', 'PHOTO', 'GEOFENCE')),
    recorded_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_delivery_outcomes_delivered_at ON delivery_outcomes (delivered_at);
CREATE INDEX idx_delivery_outcomes_carrier ON delivery_outcomes (carrier_code, delivered_at);
