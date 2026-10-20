-- @gloss represents lib:Loan
CREATE TABLE loans (id uuid PRIMARY KEY, item_id uuid, patron_id uuid);

/* @gloss represents lib:Book */
CREATE TABLE books (
  id uuid PRIMARY KEY
);
