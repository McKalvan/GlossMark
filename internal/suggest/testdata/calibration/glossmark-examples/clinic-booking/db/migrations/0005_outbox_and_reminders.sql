CREATE TABLE outbox (
    id          uuid PRIMARY KEY,
    kind        text NOT NULL,
    payload     jsonb NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    sent_at     timestamptz
);
CREATE INDEX outbox_unsent_idx ON outbox (created_at) WHERE sent_at IS NULL;

CREATE TABLE reminders (
    appointment_id  uuid NOT NULL REFERENCES appointments (id),
    patient_id      uuid NOT NULL REFERENCES patients (id),
    channel         text NOT NULL CHECK (channel IN ('sms', 'email')),
    send_at         timestamptz NOT NULL,
    template        text NOT NULL,
    sent_at         timestamptz,
    PRIMARY KEY (appointment_id, template)
);
CREATE INDEX reminders_due_idx ON reminders (send_at) WHERE sent_at IS NULL;
