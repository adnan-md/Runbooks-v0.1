-- SQLite does not support DROP COLUMN before 3.35; safe no-op for older versions.
-- For newer SQLite, this would be the down migration; left intentionally minimal.
DELETE FROM schema_migrations WHERE version='0002_job_scripts';
