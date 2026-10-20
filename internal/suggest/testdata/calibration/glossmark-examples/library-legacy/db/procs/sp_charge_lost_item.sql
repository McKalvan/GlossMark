-- sp_charge_lost_item(loan_id): bill the replacement cost of an item declared
-- lost (60 days overdue, or by the patron at the desk). Called by
-- batch/overdue_notices.pl. Replaces any overdue fine on the same loan.

CREATE OR REPLACE FUNCTION sp_charge_lost_item(p_loan_id bigint) RETURNS numeric AS $$
DECLARE
    cost numeric;
    pid  bigint;
BEGIN
    SELECT COALESCE(i.replacement_cost, 25.00), l.patron_id INTO cost, pid
      FROM loans l JOIN items i ON i.id = l.item_id
     WHERE l.id = p_loan_id;
    DELETE FROM fines WHERE loan_id = p_loan_id AND fine_type = 'OVERDUE' AND paid = 0;
    INSERT INTO fines (patron_id, loan_id, fine_type, amount, assessed_at)
    VALUES (pid, p_loan_id, 'LOST_ITEM', cost, now());
    UPDATE patrons SET balance_owed = (SELECT COALESCE(sum(amount - paid), 0) FROM fines
                                        WHERE patron_id = pid AND NOT waived)
     WHERE id = pid;
    RETURN cost;
END;
$$ LANGUAGE plpgsql;
