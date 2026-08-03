DROP INDEX idx_payments_paid_at;

ALTER TABLE payments DROP COLUMN updated_at;
ALTER TABLE payments DROP COLUMN void_reason;
ALTER TABLE payments DROP COLUMN voided_at;
ALTER TABLE payments DROP COLUMN reference;
ALTER TABLE payments DROP COLUMN method;
ALTER TABLE payments DROP COLUMN unit_price_cents;
ALTER TABLE payments DROP COLUMN device_count;
