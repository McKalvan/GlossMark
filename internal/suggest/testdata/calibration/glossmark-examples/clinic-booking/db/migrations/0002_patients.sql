-- Patient records are PII. Row access is limited to the booking_api and reminders roles.
-- @gloss represents clinic:Patient conf=0.91
CREATE TABLE patients (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    given_name     text NOT NULL,
    family_name    text NOT NULL,
    date_of_birth  date NOT NULL,
    phone          text,
    email          text,
    sms_consent    boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX patients_email_key ON patients (lower(email)) WHERE email IS NOT NULL;
