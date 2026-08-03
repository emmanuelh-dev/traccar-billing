-- Going back means one tenant per server again. If two users of the same
-- server have their own books by now, the UNIQUE below fails rather than
-- silently merging two sets of accounts into one.
CREATE TABLE tenants_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL UNIQUE,
    session_cookie TEXT NOT NULL DEFAULT '',
    session_expires_at DATETIME,
    admin_traccar_user_id INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO tenants_old (id, name, base_url, session_cookie, session_expires_at, admin_traccar_user_id, created_at, updated_at)
SELECT id, name, base_url, session_cookie, session_expires_at, admin_traccar_user_id, created_at, updated_at
FROM tenants;

DROP TABLE tenants;
ALTER TABLE tenants_old RENAME TO tenants;
