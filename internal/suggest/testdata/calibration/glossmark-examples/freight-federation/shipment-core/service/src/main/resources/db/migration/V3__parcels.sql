-- @gloss represents frt:Parcel conf=0.91
CREATE TABLE parcels (
    id            UUID    PRIMARY KEY,
    shipment_id   UUID    NOT NULL REFERENCES shipments (id) ON DELETE CASCADE,
    seq           INTEGER NOT NULL CHECK (seq > 0),
    weight_grams  INTEGER NOT NULL CHECK (weight_grams > 0),
    length_cm     INTEGER NOT NULL CHECK (length_cm > 0),
    width_cm      INTEGER NOT NULL CHECK (width_cm > 0),
    height_cm     INTEGER NOT NULL CHECK (height_cm > 0),
    UNIQUE (shipment_id, seq)
);

CREATE INDEX idx_parcels_shipment ON parcels (shipment_id);
