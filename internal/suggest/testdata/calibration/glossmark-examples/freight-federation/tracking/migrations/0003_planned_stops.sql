-- Planned stops from RoutePlanned on dispatch.routes.v1. A re-planned route
-- replaces its stops wholesale.
-- @gloss represents frt:Route conf=0.45
CREATE TABLE planned_stops (
    route_id     UUID        NOT NULL,
    shipment_id  UUID        NOT NULL,
    seq          INTEGER     NOT NULL,
    driver_id    UUID        NOT NULL,
    route_date   DATE        NOT NULL,
    lat          DOUBLE PRECISION NOT NULL,
    lng          DOUBLE PRECISION NOT NULL,
    eta_from     TIMESTAMPTZ NOT NULL,
    eta_to       TIMESTAMPTZ NOT NULL,
    planned_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (route_id, shipment_id),
    CHECK (eta_to > eta_from)
);

CREATE INDEX planned_stops_shipment_idx ON planned_stops (shipment_id, planned_at DESC);
