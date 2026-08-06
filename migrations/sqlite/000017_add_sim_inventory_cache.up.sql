CREATE TABLE sim_inventory_cache (
    tenant_id INTEGER PRIMARY KEY REFERENCES tenants(id),
    payload TEXT NOT NULL,
    refreshed_at DATETIME NOT NULL
);
