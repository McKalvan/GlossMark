-- @gloss represents lib:Patron
CREATE TABLE patrons (id uuid PRIMARY KEY, card_number text UNIQUE, name text);

-- @gloss represents lib:Loan
CREATE TABLE loans (id uuid PRIMARY KEY, item_id uuid, patron_id uuid REFERENCES patrons, due_on date);
