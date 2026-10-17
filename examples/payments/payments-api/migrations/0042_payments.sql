-- migrations/0042_payments.sql
-- @gloss represents pay:Payment
CREATE TABLE payments (id uuid PRIMARY KEY, merchant_id uuid, amount_minor bigint, status text);
