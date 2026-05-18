-- +goose Down
DROP TABLE IF EXISTS course_progress;
DROP TABLE IF EXISTS course_entitlements;
DROP TABLE IF EXISTS courses;
DROP TABLE IF EXISTS person_emails;
DROP TABLE IF EXISTS people;
