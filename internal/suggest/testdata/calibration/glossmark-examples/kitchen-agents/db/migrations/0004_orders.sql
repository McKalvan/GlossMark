-- @gloss represents dine:Order conf=0.91
CREATE TABLE orders (
    id           uuid PRIMARY KEY,
    location_id  uuid NOT NULL REFERENCES locations (id),
    table_label  text NOT NULL,
    status       text NOT NULL CHECK (status IN ('open', 'fired', 'served', 'paid', 'void')),
    total        numeric(10, 2) NOT NULL DEFAULT 0,
    document     jsonb NOT NULL,
    opened_at    timestamptz NOT NULL
);

CREATE INDEX orders_open ON orders (location_id, status) WHERE status IN ('open', 'fired', 'served');
