CREATE TABLE sim_inventory_cache (
    tenant_id BIGINT NOT NULL PRIMARY KEY,
    payload LONGTEXT NOT NULL,
    refreshed_at DATETIME NOT NULL,
    CONSTRAINT fk_sim_inventory_cache_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);
