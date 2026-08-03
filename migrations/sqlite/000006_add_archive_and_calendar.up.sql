ALTER TABLE accounts ADD COLUMN archived_at DATETIME;

ALTER TABLE subscriptions ADD COLUMN billing_mode TEXT NOT NULL DEFAULT 'rolling';
ALTER TABLE subscriptions ADD COLUMN anchor_day INTEGER NOT NULL DEFAULT 1;
ALTER TABLE subscriptions ADD COLUMN due_day INTEGER NOT NULL DEFAULT 5;

CREATE INDEX idx_accounts_archived ON accounts(tenant_id, archived_at);
