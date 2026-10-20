-- sp_assess_fines(as_of): accrue overdue fines for loans still out.
-- Called nightly by batch/assess_fines.rb. Mirrors FineCalculator.java:
-- grace period, daily rate by format, juveniles exempt, seniors half, capped
-- at the lower of the item's replacement cost and the flat cap.

-- @gloss implements .NightlyFineJob conf=0.85
CREATE OR REPLACE FUNCTION sp_assess_fines(as_of date)
RETURNS TABLE (loans_charged integer, total_charged numeric) AS $$
DECLARE
    -- @gloss implements .GracePeriodRule scope=line conf=0.8
    grace      constant integer := 3;
    -- @gloss implements .FineCapRule scope=line conf=0.8
    flat_cap   constant numeric := 20.00;
BEGIN
    WITH late AS (
        SELECT l.id AS loan_id, l.patron_id, i.format, i.replacement_cost, p.patron_type,
               (as_of - l.due_date) - grace AS days_late
          FROM loans l
          JOIN items i   ON i.id = l.item_id
          JOIN patrons p ON p.id = l.patron_id
         WHERE l.status IN ('ACTIVE','OVERDUE')
           AND as_of - l.due_date > grace
           AND p.patron_type <> 'JUVENILE'
    ), priced AS (
        SELECT loan_id, patron_id,
               LEAST(days_late * CASE format WHEN 'DVD' THEN 1.00 WHEN 'MAGAZINE' THEN 0.10 ELSE 0.25 END
                               * CASE patron_type WHEN 'SENIOR' THEN 0.5 ELSE 1 END,
                     COALESCE(LEAST(replacement_cost, flat_cap), flat_cap)) AS amount
          FROM late
    ), upserted AS (
        INSERT INTO fines (patron_id, loan_id, fine_type, amount, assessed_at)
        SELECT patron_id, loan_id, 'OVERDUE', amount, now() FROM priced
        ON CONFLICT (loan_id) WHERE fine_type = 'OVERDUE'
        DO UPDATE SET amount = EXCLUDED.amount, assessed_at = EXCLUDED.assessed_at
        RETURNING patron_id, amount
    )
    SELECT count(*)::integer, COALESCE(sum(amount), 0) INTO loans_charged, total_charged FROM upserted;

    UPDATE patrons p
       SET balance_owed = (SELECT COALESCE(sum(f.amount - f.paid), 0) FROM fines f
                            WHERE f.patron_id = p.id AND NOT f.waived);
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql;
