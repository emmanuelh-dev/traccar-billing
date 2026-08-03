-- A tenant used to reach Traccar only with the JSESSIONID captured at login.
-- That cookie expires, and when it does the scheduler silently stops being able
-- to suspend anyone: billing kept running while enforcement quietly did not.
-- A Traccar API token does not expire on its own, so the background job no
-- longer depends on someone opening the web UI often enough.
ALTER TABLE tenants ADD COLUMN api_token TEXT NOT NULL DEFAULT '';
