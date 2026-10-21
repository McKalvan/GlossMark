CREATE TABLE patrons (
    id integer PRIMARY KEY,
    card_number text NOT NULL,
    branch_id integer REFERENCES branches(id)
);
ALTER TABLE borrowers RENAME TO patrons;
