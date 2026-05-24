DROP INDEX IF EXISTS course_attempts_version_idx;
ALTER TABLE course_attempts DROP COLUMN IF EXISTS course_version_id;
DROP INDEX IF EXISTS course_versions_latest_idx;
DROP INDEX IF EXISTS course_versions_published_hash_idx;
DROP TABLE IF EXISTS course_versions;
