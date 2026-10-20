CREATE TYPE appointment_status AS ENUM ('booked', 'cancelled', 'attended', 'no_show');

-- @gloss represents clinic:Appointment conf=0.91
CREATE TABLE appointments (
    id               uuid PRIMARY KEY,
    patient_id       uuid NOT NULL REFERENCES patients (id),
    practitioner_id  uuid NOT NULL REFERENCES practitioners (id),
    slot_id          uuid NOT NULL REFERENCES slots (id),
    starts_at        timestamptz NOT NULL,
    reason           text,
    status           appointment_status NOT NULL DEFAULT 'booked',
    cancelled_at     timestamptz,
    late_cancel      boolean NOT NULL DEFAULT false,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX appointments_live_slot_key ON appointments (slot_id) WHERE status = 'booked';
CREATE INDEX appointments_patient_idx ON appointments (patient_id, starts_at);
