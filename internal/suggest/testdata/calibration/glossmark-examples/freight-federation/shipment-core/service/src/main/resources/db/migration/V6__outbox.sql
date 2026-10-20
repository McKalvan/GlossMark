-- @gloss represents .Outbox conf=0.64
CREATE TABLE outbox_events (
    id              UUID          PRIMARY KEY,
    aggregate_type  VARCHAR(64)   NOT NULL,
    aggregate_id    UUID          NOT NULL,
    event_type      VARCHAR(64)   NOT NULL,
    topic           VARCHAR(128)  NOT NULL,
    payload         JSONB         NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    sent_at         TIMESTAMPTZ,
    attempts        INTEGER       NOT NULL DEFAULT 0,
    last_error      VARCHAR(1024)
);

CREATE INDEX idx_outbox_events_unsent ON outbox_events (created_at) WHERE sent_at IS NULL;
CREATE INDEX idx_outbox_events_aggregate ON outbox_events (aggregate_type, aggregate_id);
