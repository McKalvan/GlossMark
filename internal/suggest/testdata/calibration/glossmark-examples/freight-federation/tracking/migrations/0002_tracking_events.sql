-- GPS pings from driver devices and scan events from depot and handheld
-- scanners share one append-only table, distinguished by `kind`.
-- @gloss represents .TrackingStore conf=0.36
CREATE TABLE tracking_events (
    id               BIGSERIAL PRIMARY KEY,
    kind             TEXT        NOT NULL CHECK (kind IN ('GPS_PING', 'SCAN')),
    shipment_id      UUID        REFERENCES tracked_shipments (shipment_id),
    tracking_number  TEXT,
    driver_id        UUID,
    device_id        TEXT        NOT NULL,
    lat              DOUBLE PRECISION,
    lng              DOUBLE PRECISION,
    accuracy_m       REAL,
    scan_type        TEXT CHECK (scan_type IN ('DEPOT_IN', 'DEPOT_OUT', 'OUT_FOR_DELIVERY', 'DELIVERED', 'EXCEPTION')),
    scan_location    TEXT,
    raw_barcode      TEXT,
    recorded_at      TIMESTAMPTZ NOT NULL,
    received_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ping_has_position CHECK (kind <> 'GPS_PING' OR (lat IS NOT NULL AND lng IS NOT NULL AND driver_id IS NOT NULL)),
    CONSTRAINT scan_has_barcode  CHECK (kind <> 'SCAN' OR (raw_barcode IS NOT NULL AND scan_type IS NOT NULL))
);

CREATE INDEX tracking_events_shipment_idx ON tracking_events (shipment_id, recorded_at DESC);
CREATE INDEX tracking_events_driver_idx   ON tracking_events (driver_id, recorded_at DESC) WHERE kind = 'GPS_PING';
CREATE UNIQUE INDEX tracking_events_dedup_idx ON tracking_events (device_id, kind, recorded_at);
