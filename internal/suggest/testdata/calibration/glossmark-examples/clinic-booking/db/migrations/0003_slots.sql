-- @gloss represents clinic:Slot conf=0.91
CREATE TABLE slots (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    practitioner_id  uuid NOT NULL REFERENCES practitioners (id),
    clinic_id        uuid NOT NULL REFERENCES clinics (id),
    starts_at        timestamptz NOT NULL,
    ends_at          timestamptz NOT NULL,
    booked           boolean NOT NULL DEFAULT false,
    CHECK (ends_at > starts_at)
);

CREATE INDEX slots_open_idx ON slots (practitioner_id, starts_at) WHERE NOT booked;
