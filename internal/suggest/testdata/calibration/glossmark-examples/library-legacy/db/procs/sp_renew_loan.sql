-- sp_renew_loan(loan_id, period_days, OUT new_due): extend a loan by one
-- period from today. RenewalService checks the rules first; this re-checks the
-- two that can race (a hold placed meanwhile, a concurrent renewal).

-- @gloss implements .RenewalHandler conf=0.8
CREATE OR REPLACE FUNCTION sp_renew_loan(p_loan_id bigint, p_period_days integer, OUT new_due date) AS $$
DECLARE
    l record;
BEGIN
    SELECT lo.id, lo.renewals, i.title_id INTO l
      FROM loans lo JOIN items i ON i.id = lo.item_id
     WHERE lo.id = p_loan_id AND lo.status IN ('ACTIVE','OVERDUE')
     FOR UPDATE OF lo;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'NOT_OUT';
    END IF;
    IF l.renewals >= 2 THEN
        RAISE EXCEPTION 'MAX_RENEWALS';
    END IF;
    IF EXISTS (SELECT 1 FROM holds WHERE title_id = l.title_id AND state = 'WAITING') THEN
        RAISE EXCEPTION 'ON_HOLD';
    END IF;
    new_due := current_date + p_period_days;
    UPDATE loans SET due_date = new_due, renewals = renewals + 1, status = 'ACTIVE' WHERE id = p_loan_id;
END;
$$ LANGUAGE plpgsql;
