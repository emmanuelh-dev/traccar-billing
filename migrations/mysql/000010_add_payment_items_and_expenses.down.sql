DROP TABLE IF EXISTS expenses;
DROP TABLE IF EXISTS payment_items;

DELETE FROM payments WHERE subscription_id IS NULL;

ALTER TABLE payments DROP FOREIGN KEY fk_payments_account;
ALTER TABLE payments DROP INDEX idx_payments_account;
ALTER TABLE payments MODIFY COLUMN subscription_id BIGINT NOT NULL;
ALTER TABLE payments DROP COLUMN account_id;
