CREATE TABLE sellers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    commission_bp INTEGER NOT NULL DEFAULT 0,
    active INTEGER NOT NULL DEFAULT 1,
    note TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sellers_tenant ON sellers(tenant_id, active);

ALTER TABLE accounts ADD COLUMN seller_id INTEGER REFERENCES sellers(id);

CREATE INDEX idx_accounts_seller ON accounts(seller_id);
