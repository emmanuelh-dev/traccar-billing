ALTER TABLE tenants ADD COLUMN connectivity_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE tenants ADD COLUMN connectivity_token TEXT NOT NULL DEFAULT '';
