-- @gloss represents dine:Station conf=0.8
CREATE TYPE kitchen_station AS ENUM ('grill', 'fry', 'saute', 'cold', 'pastry', 'pass');

-- @gloss represents dine:MenuItem conf=0.91
CREATE TABLE menu_items (
    id           uuid PRIMARY KEY,
    location_id  uuid NOT NULL REFERENCES locations (id),
    section      text NOT NULL,
    position     integer NOT NULL DEFAULT 0,
    name         text NOT NULL CHECK (length(name) <= 80),
    description  text NOT NULL DEFAULT '',
    price        numeric(8, 2) NOT NULL CHECK (price > 0),
    station      kitchen_station NOT NULL,
    available    boolean NOT NULL DEFAULT true,
    retired_at   timestamptz,
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX menu_items_location ON menu_items (location_id) WHERE retired_at IS NULL;
