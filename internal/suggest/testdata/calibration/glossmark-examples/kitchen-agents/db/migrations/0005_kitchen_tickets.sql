-- @gloss represents dine:KitchenTicket conf=0.91
CREATE TABLE kitchen_tickets (
    id              uuid PRIMARY KEY,
    order_id        uuid NOT NULL REFERENCES orders (id),
    station         kitchen_station NOT NULL,
    table_label     text NOT NULL,
    lines           jsonb NOT NULL,
    allergen_alert  allergen[] NOT NULL DEFAULT '{}',
    fired_at        timestamptz NOT NULL,
    bumped_at       timestamptz
);

CREATE INDEX kitchen_tickets_open ON kitchen_tickets (station, fired_at) WHERE bumped_at IS NULL;
