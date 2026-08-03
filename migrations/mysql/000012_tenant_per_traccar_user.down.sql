-- Going back means one tenant per server again. If two users of the same
-- server have their own books by now, the UNIQUE below fails rather than
-- silently merging two sets of accounts into one.
ALTER TABLE tenants
    DROP INDEX uq_tenants_owner,
    ADD UNIQUE KEY uq_tenants_base_url (base_url);

ALTER TABLE tenants
    DROP COLUMN traccar_user_id,
    DROP COLUMN owner_email;
