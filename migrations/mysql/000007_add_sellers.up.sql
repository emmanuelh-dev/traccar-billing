CREATE TABLE sellers (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(191) NOT NULL,
    email VARCHAR(191) NOT NULL DEFAULT '',
    phone VARCHAR(64) NOT NULL DEFAULT '',
    commission_bp INT NOT NULL DEFAULT 0,
    active TINYINT(1) NOT NULL DEFAULT 1,
    note VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_sellers_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

CREATE INDEX idx_sellers_tenant ON sellers(tenant_id, active);

ALTER TABLE accounts ADD COLUMN seller_id BIGINT NULL;
ALTER TABLE accounts ADD CONSTRAINT fk_accounts_seller FOREIGN KEY (seller_id) REFERENCES sellers(id);
