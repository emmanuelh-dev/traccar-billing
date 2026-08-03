CREATE TABLE tenant_settings (
    tenant_id INTEGER PRIMARY KEY REFERENCES tenants(id),
    billing_mode TEXT NOT NULL DEFAULT 'rolling',
    anchor_day INTEGER NOT NULL DEFAULT 1,
    due_day INTEGER NOT NULL DEFAULT 5,
    period_days INTEGER NOT NULL DEFAULT 30,
    grace_days INTEGER NOT NULL DEFAULT 5,
    currency TEXT NOT NULL DEFAULT 'MXN',
    unit_price_cents INTEGER NOT NULL DEFAULT 0,
    flat_fee_cents INTEGER NOT NULL DEFAULT 0,
    min_devices INTEGER NOT NULL DEFAULT 0,
    hide_mirror_accounts INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO tenant_settings (tenant_id) SELECT id FROM tenants;
