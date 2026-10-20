-- 003: items, one row per physical copy.

-- @gloss represents lib:Item conf=0.91
CREATE TABLE items (
    id                bigserial PRIMARY KEY,
    barcode           varchar(14)  NOT NULL UNIQUE,
    title_id          bigint       NOT NULL REFERENCES titles (id),
    branch_id         integer      NOT NULL REFERENCES branches (id),
    format            varchar(12)  NOT NULL CHECK (format IN ('BOOK','LARGE_PRINT','DVD','AUDIOBOOK','MAGAZINE')),
    call_number       varchar(40),
    replacement_cost  numeric(6,2),
    is_reference      boolean      NOT NULL DEFAULT false
);

CREATE INDEX items_title_idx ON items (title_id);
