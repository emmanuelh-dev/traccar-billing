DROP INDEX idx_accounts_archived ON accounts;

ALTER TABLE subscriptions DROP COLUMN due_day;
ALTER TABLE subscriptions DROP COLUMN anchor_day;
ALTER TABLE subscriptions DROP COLUMN billing_mode;

ALTER TABLE accounts DROP COLUMN archived_at;
