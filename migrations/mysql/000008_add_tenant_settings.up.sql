CREATE TABLE tenant_settings (
    tenant_id BIGINT NOT NULL PRIMARY KEY,
    billing_mode VARCHAR(16) NOT NULL DEFAULT 'rolling',
    anchor_day INT NOT NULL DEFAULT 1,
    due_day INT NOT NULL DEFAULT 5,
    period_days INT NOT NULL DEFAULT 30,
    grace_days INT NOT NULL DEFAULT 5,
    currency VARCHAR(8) NOT NULL DEFAULT 'MXN',
    unit_price_cents BIGINT NOT NULL DEFAULT 0,
    flat_fee_cents BIGINT NOT NULL DEFAULT 0,
    min_devices INT NOT NULL DEFAULT 0,
    hide_mirror_accounts TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_tenant_settings_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

INSERT INTO tenant_settings (tenant_id) SELECT id FROM tenants;
