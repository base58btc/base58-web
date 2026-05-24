CREATE TABLE course_version_files (
  id BIGSERIAL PRIMARY KEY,
  course_version_id BIGINT NOT NULL REFERENCES course_versions(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (course_version_id, path)
);

CREATE INDEX course_version_files_version_idx ON course_version_files(course_version_id);
