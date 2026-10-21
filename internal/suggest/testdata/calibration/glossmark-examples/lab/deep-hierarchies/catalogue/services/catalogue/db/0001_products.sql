-- 0001_products.sql: the product table and the brake detail table.

-- @gloss represents cyc:Product
CREATE TABLE products (
    sku          text PRIMARY KEY,
    name         text NOT NULL,
    category     text NOT NULL,
    price_cents  integer NOT NULL CHECK (price_cents >= 0),
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- @gloss represents cyc:HydraulicDiscBrake
CREATE TABLE hydraulic_disc_brakes (
    sku           text PRIMARY KEY REFERENCES products (sku),
    fluid         text NOT NULL CHECK (fluid IN ('mineral', 'dot')),
    max_rotor_mm  integer NOT NULL
);
