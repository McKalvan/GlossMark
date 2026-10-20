-- 004: loans. One row per checkout; the row stays after check-in for statistics.

-- @gloss represents lib:Loan conf=0.91
CREATE TABLE loans (
    id              bigserial PRIMARY KEY,
    patron_id       bigint      NOT NULL REFERENCES patrons (id),
    item_id         bigint      NOT NULL REFERENCES items (id),
    branch_id       integer     NOT NULL REFERENCES branches (id),
    checked_out_at  timestamp   NOT NULL,
    due_date        date        NOT NULL,
    returned_at     timestamp,
    renewals        smallint    NOT NULL DEFAULT 0,
    status          varchar(10) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','OVERDUE','RETURNED','LOST'))
);

CREATE UNIQUE INDEX loans_one_open_per_item ON loans (item_id) WHERE status IN ('ACTIVE','OVERDUE');
CREATE INDEX loans_patron_idx ON loans (patron_id) WHERE status IN ('ACTIVE','OVERDUE');
