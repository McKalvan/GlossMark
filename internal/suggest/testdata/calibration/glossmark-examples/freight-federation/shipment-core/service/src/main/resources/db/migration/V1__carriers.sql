-- @gloss represents frt:Carrier conf=0.91
CREATE TABLE carriers (
    code                     VARCHAR(16)  PRIMARY KEY,
    name                     VARCHAR(120) NOT NULL,
    postcode_prefixes        TEXT[]       NOT NULL DEFAULT '{}',
    max_parcel_weight_grams  INTEGER      NOT NULL CHECK (max_parcel_weight_grams > 0),
    supports_express         BOOLEAN      NOT NULL DEFAULT FALSE,
    priority                 INTEGER      NOT NULL DEFAULT 100,
    active                   BOOLEAN      NOT NULL DEFAULT TRUE
);

CREATE INDEX idx_carriers_active_priority ON carriers (priority, code) WHERE active;
CREATE INDEX idx_carriers_postcode_prefixes ON carriers USING GIN (postcode_prefixes);
