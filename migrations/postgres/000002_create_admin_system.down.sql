DROP INDEX IF EXISTS admin_audit_events_target_idx;
DROP INDEX IF EXISTS admin_audit_events_created_idx;
DROP TABLE IF EXISTS admin_audit_events;
ALTER TABLE courses DROP COLUMN IF EXISTS header_img;
ALTER TABLE courses DROP COLUMN IF EXISTS description;
