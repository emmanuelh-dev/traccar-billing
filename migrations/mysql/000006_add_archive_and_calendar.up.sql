ALTER TABLE accounts ADD COLUMN archived_at DATETIME NULL;

ALTER TABLE subscriptions ADD COLUMN billing_mode VARCHAR(16) NOT NULL DEFAULT 'rolling';
ALTER TABLE subscriptions ADD COLUMN anchor_day INT NOT NULL DEFAULT 1;
ALTER TABLE subscriptions ADD COLUMN due_day INT NOT NULL DEFAULT 5;

CREATE INDEX idx_accounts_archived ON accounts(tenant_id, archived_at);
