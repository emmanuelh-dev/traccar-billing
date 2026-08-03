-- See the sqlite counterpart: the login cookie expires and takes automatic
-- suspension down with it, so the tenant stores a Traccar API token instead.
ALTER TABLE tenants ADD COLUMN api_token VARCHAR(500) NOT NULL DEFAULT '';
