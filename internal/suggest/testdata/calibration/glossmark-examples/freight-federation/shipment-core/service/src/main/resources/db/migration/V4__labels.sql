-- @gloss represents frt:ShippingLabel conf=0.91
CREATE TABLE shipping_labels (
    id               UUID         PRIMARY KEY,
    shipment_id      UUID         NOT NULL UNIQUE REFERENCES shipments (id) ON DELETE CASCADE,
    tracking_number  VARCHAR(13)  NOT NULL,
    format           VARCHAR(64)  NOT NULL,
    zpl              TEXT         NOT NULL,
    rendered_at      TIMESTAMPTZ  NOT NULL
);

CREATE INDEX idx_shipping_labels_tracking_number ON shipping_labels (tracking_number);
