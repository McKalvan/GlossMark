-- CIRC schema, PostgreSQL. Applied in order by db/apply.sh; never edited after release.
-- 001: branches and bibliographic titles (titles are loaded nightly from the catalogue).

-- @gloss represents lib:Branch conf=0.91
CREATE TABLE branches (
    id          integer PRIMARY KEY,
    code        varchar(8)  NOT NULL UNIQUE,
    name        varchar(80) NOT NULL,
    address     varchar(200),
    mail_run    boolean     NOT NULL DEFAULT false
);

-- @gloss represents lib:Title conf=0.91
CREATE TABLE titles (
    id          bigserial PRIMARY KEY,
    marc_001    varchar(40) NOT NULL UNIQUE,
    title       varchar(500) NOT NULL,
    author      varchar(200),
    isbn        varchar(17),
    pub_year    smallint
);

CREATE INDEX titles_isbn_idx ON titles (isbn);
