-- A tenant used to be a Traccar server, so every user of that server shared
-- one set of books: the same accounts, payments, expenses and agenda. A tenant
-- is now a Traccar *user*, so whoever logs in only ever sees their own.
ALTER TABLE tenants
    ADD COLUMN traccar_user_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN owner_email VARCHAR(191) NOT NULL DEFAULT '';

-- Existing books stay with the last user recorded as their admin, the only
-- owner the old schema ever stored.
UPDATE tenants SET traccar_user_id = admin_traccar_user_id;

ALTER TABLE tenants
    DROP INDEX uq_tenants_base_url,
    ADD UNIQUE KEY uq_tenants_owner (base_url, traccar_user_id);
