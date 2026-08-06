ALTER TABLE tenants
    ADD COLUMN connectivity_provider VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN connectivity_token VARCHAR(2048) NOT NULL DEFAULT '';
