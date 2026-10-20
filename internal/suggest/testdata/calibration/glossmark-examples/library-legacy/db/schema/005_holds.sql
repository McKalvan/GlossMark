-- 005: title-level holds (2012).

-- @gloss represents lib:Hold conf=0.91
CREATE TABLE holds (
    id                bigserial PRIMARY KEY,
    patron_id         bigint      NOT NULL REFERENCES patrons (id),
    title_id          bigint      NOT NULL REFERENCES titles (id),
    pickup_branch_id  integer     NOT NULL REFERENCES branches (id),
    placed_at         timestamp   NOT NULL,
    state             varchar(8)  NOT NULL DEFAULT 'WAITING' CHECK (state IN ('WAITING','TRAPPED','FILLED','EXPIRED')),
    trapped_item_id   bigint      REFERENCES items (id),
    trapped_at        timestamp,
    pickup_by         timestamp
);

CREATE INDEX holds_queue_idx ON holds (title_id, placed_at) WHERE state = 'WAITING';
