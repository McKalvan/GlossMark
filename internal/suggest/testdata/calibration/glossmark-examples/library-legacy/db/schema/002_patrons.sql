-- 002: patrons. Renamed from borrowers in 3.2 (2016, CIRC-412); 007 keeps
-- a borrowers view with the old shape.

-- @gloss represents lib:Patron conf=0.91
CREATE TABLE patrons (
    id              bigserial PRIMARY KEY,
    card_number     varchar(14)  NOT NULL UNIQUE,
    last_name       varchar(80)  NOT NULL,
    first_name      varchar(80)  NOT NULL,
    email           varchar(254),
    phone           varchar(20),
    patron_type     varchar(12)  NOT NULL CHECK (patron_type IN ('ADULT','JUVENILE','SENIOR','NONRESIDENT')),
    home_branch_id  integer      NOT NULL REFERENCES branches (id),
    expires_on      date         NOT NULL,
    balance_owed    numeric(8,2) NOT NULL DEFAULT 0,
    email_notices   boolean      NOT NULL DEFAULT true,
    last_activity   date
);
