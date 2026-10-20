-- sp_expire_holds(as_of): expire trapped holds past pickup_by and re-trap the
-- copy for the next waiting hold on the title. Called by batch/bin/expire_holds.sh.

-- @gloss implements .HoldExpiryJob conf=0.8
CREATE OR REPLACE FUNCTION sp_expire_holds(as_of date)
RETURNS TABLE (expired integer, retrapped integer) AS $$
DECLARE
    h       record;
    nxt     bigint;
BEGIN
    expired := 0;
    retrapped := 0;
    FOR h IN SELECT id, title_id, trapped_item_id FROM holds
              WHERE state = 'TRAPPED' AND pickup_by < as_of FOR UPDATE LOOP
        UPDATE holds SET state = 'EXPIRED' WHERE id = h.id;
        expired := expired + 1;
        SELECT id INTO nxt FROM holds
         WHERE title_id = h.title_id AND state = 'WAITING'
         ORDER BY placed_at, id LIMIT 1 FOR UPDATE;
        IF FOUND THEN
            UPDATE holds SET state = 'TRAPPED', trapped_item_id = h.trapped_item_id,
                             trapped_at = now(), pickup_by = as_of + 7
             WHERE id = nxt;
            retrapped := retrapped + 1;
        END IF;
    END LOOP;
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql;
