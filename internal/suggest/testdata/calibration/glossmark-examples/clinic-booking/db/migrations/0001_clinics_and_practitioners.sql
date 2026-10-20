CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- @gloss represents clinic:Clinic conf=0.91
CREATE TABLE clinics (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    timezone    text NOT NULL DEFAULT 'Europe/London',
    address     text NOT NULL
);

-- @gloss represents clinic:Practitioner conf=0.91
CREATE TABLE practitioners (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name            text NOT NULL,
    specialty               text NOT NULL,
    accepting_new_patients  boolean NOT NULL DEFAULT true
);

CREATE TABLE practitioner_clinics (
    practitioner_id uuid NOT NULL REFERENCES practitioners (id),
    clinic_id       uuid NOT NULL REFERENCES clinics (id),
    PRIMARY KEY (practitioner_id, clinic_id)
);
