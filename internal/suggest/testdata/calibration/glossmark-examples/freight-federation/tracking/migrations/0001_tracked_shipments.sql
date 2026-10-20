-- @gloss represents frt:Shipment conf=0.45
CREATE TABLE tracked_shipments (
    shipment_id      UUID PRIMARY KEY,
    tracking_number  TEXT        NOT NULL UNIQUE,
    driver_id        UUID        NOT NULL,
    route_id         UUID        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'IN_TRANSIT'
                     CHECK (status IN ('IN_TRANSIT', 'ARRIVED', 'DELIVERED')),
    picked_up_at     TIMESTAMPTZ NOT NULL,
    pickup_lat       DOUBLE PRECISION NOT NULL,
    pickup_lng       DOUBLE PRECISION NOT NULL,
    last_lat         DOUBLE PRECISION,
    last_lng         DOUBLE PRECISION,
    last_seen_at     TIMESTAMPTZ,
    arrived_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX tracked_shipments_driver_active_idx
    ON tracked_shipments (driver_id)
    WHERE status <> 'DELIVERED';
