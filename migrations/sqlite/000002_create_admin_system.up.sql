ALTER TABLE courses ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE courses ADD COLUMN header_img TEXT NOT NULL DEFAULT '';

CREATE TABLE admin_audit_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL DEFAULT '',
  details TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX admin_audit_events_created_idx ON admin_audit_events(created_at);
CREATE INDEX admin_audit_events_target_idx ON admin_audit_events(target_type, target_id);
