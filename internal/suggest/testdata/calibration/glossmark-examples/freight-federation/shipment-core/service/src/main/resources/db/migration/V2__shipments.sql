-- @gloss represents frt:Shipment conf=0.91
CREATE TABLE shipments (
    id                UUID         PRIMARY KEY,
    tracking_number   VARCHAR(13)  NOT NULL UNIQUE CHECK (tracking_number ~ '^KF[0-9]{11}$'),
    shipper_id        VARCHAR(64)  NOT NULL,
    pickup_line1      VARCHAR(120) NOT NULL,
    pickup_city       VARCHAR(80)  NOT NULL,
    pickup_postcode   VARCHAR(12)  NOT NULL,
    pickup_country    VARCHAR(2)   NOT NULL,
    pickup_lat        DOUBLE PRECISION NOT NULL,
    pickup_lng        DOUBLE PRECISION NOT NULL,
    delivery_line1    VARCHAR(120) NOT NULL,
    delivery_city     VARCHAR(80)  NOT NULL,
    delivery_postcode VARCHAR(12)  NOT NULL,
    delivery_country  VARCHAR(2)   NOT NULL,
    delivery_lat      DOUBLE PRECISION NOT NULL,
    delivery_lng      DOUBLE PRECISION NOT NULL,
    service_level     VARCHAR(16)  NOT NULL CHECK (service_level IN ('STANDARD', 'EXPRESS')),
    status            VARCHAR(16)  NOT NULL CHECK (status IN ('BOOKED', 'PICKED_UP', 'DELIVERED', 'CANCELLED')),
    carrier_code      VARCHAR(16)  NOT NULL REFERENCES carriers (code),
    consignment_id    UUID,
    window_start      TIMESTAMPTZ  NOT NULL,
    window_end        TIMESTAMPTZ  NOT NULL,
    booked_at         TIMESTAMPTZ  NOT NULL,
    picked_up_at      TIMESTAMPTZ,
    delivered_at      TIMESTAMPTZ,
    version           BIGINT       NOT NULL DEFAULT 0,
    CHECK (window_end > window_start)
);

CREATE INDEX idx_shipments_shipper ON shipments (shipper_id, booked_at DESC);
CREATE INDEX idx_shipments_carrier_status ON shipments (carrier_code, status);
CREATE INDEX idx_shipments_window_start ON shipments (window_start);
