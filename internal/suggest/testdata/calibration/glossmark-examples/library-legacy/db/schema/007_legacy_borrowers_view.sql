-- 007: the pre-2016 borrowers shape, as a view over patrons (CIRC-412).
-- Only BorrowerActivityReport and the state annual-report export read it.
-- Drop it when the state library accepts "patron" in its template.

-- @gloss represents lib:Borrower conf=0.9
CREATE VIEW borrowers AS
SELECT p.id                                   AS borrower_id,
       p.card_number                          AS barcode,
       p.last_name || ', ' || p.first_name    AS borrower_name,
       p.patron_type                          AS borrower_class,
       (SELECT count(*) FROM loans l
         WHERE l.patron_id = p.id AND l.status IN ('ACTIVE','OVERDUE')) AS items_out,
       p.last_activity
  FROM patrons p;
