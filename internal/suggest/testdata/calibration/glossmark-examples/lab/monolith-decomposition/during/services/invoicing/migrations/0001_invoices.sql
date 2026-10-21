-- @gloss represents hire:Invoice
CREATE TABLE invoices (
    id          BIGSERIAL PRIMARY KEY,
    hire_id     BIGINT      NOT NULL UNIQUE,
    net_pence   BIGINT      NOT NULL CHECK (net_pence >= 0),
    vat_pence   BIGINT      NOT NULL CHECK (vat_pence >= 0),
    issued_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
