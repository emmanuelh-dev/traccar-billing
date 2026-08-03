-- A tenant used to be a Traccar server, so every user of that server shared
-- one set of books: the same accounts, payments, expenses and agenda. A tenant
-- is now a Traccar *user*, so whoever logs in only ever sees their own.

-- base_url carries an inline UNIQUE that SQLite cannot drop in place, so the
-- table is rebuilt. It is dropped before the replacement takes its name, which
-- leaves the REFERENCES tenants(id) clauses in the other tables pointing at
-- the new table instead of at a renamed leftover.
CREATE TABLE tenants_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    session_cookie TEXT NOT NULL DEFAULT '',
    session_expires_at DATETIME,
    admin_traccar_user_id INTEGER NOT NULL DEFAULT 0,
    traccar_user_id INTEGER NOT NULL DEFAULT 0,
    owner_email TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Existing books stay with the last user recorded as their admin, the only
-- owner the old schema ever stored.
INSERT INTO tenants_new (id, name, base_url, session_cookie, session_expires_at, admin_traccar_user_id, traccar_user_id, owner_email, created_at, updated_at)
SELECT id, name, base_url, session_cookie, session_expires_at, admin_traccar_user_id, admin_traccar_user_id, '', created_at, updated_at
FROM tenants;

DROP TABLE tenants;
ALTER TABLE tenants_new RENAME TO tenants;

CREATE UNIQUE INDEX uq_tenants_owner ON tenants(base_url, traccar_user_id);
