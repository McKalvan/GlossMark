-- @gloss represents dine:Payment conf=0.45
CREATE TABLE payment_intents (
    id            uuid PRIMARY KEY,
    order_id      uuid NOT NULL REFERENCES orders (id),
    amount        numeric(10, 2) NOT NULL,
    tip           numeric(10, 2) NOT NULL DEFAULT 0,
    currency      char(3) NOT NULL DEFAULT 'GBP',
    provider_ref  text UNIQUE,
    status        text NOT NULL CHECK (status IN ('pending', 'authorised', 'captured', 'failed')),
    created_at    timestamptz NOT NULL DEFAULT now()
);
