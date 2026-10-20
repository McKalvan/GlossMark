-- 006: fines. A fine row per charge; payments reduce paid, never delete rows.

-- @gloss represents lib:Fine conf=0.55
CREATE TABLE fines (
    id           bigserial PRIMARY KEY,
    patron_id    bigint       NOT NULL REFERENCES patrons (id),
    loan_id      bigint       REFERENCES loans (id),
    fine_type    varchar(10)  NOT NULL CHECK (fine_type IN ('OVERDUE','LOST_ITEM','DAMAGE')),
    amount       numeric(8,2) NOT NULL CHECK (amount >= 0),
    paid         numeric(8,2) NOT NULL DEFAULT 0,
    assessed_at  timestamp    NOT NULL,
    waived       boolean      NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX fines_one_overdue_per_loan ON fines (loan_id) WHERE fine_type = 'OVERDUE';
