CREATE TABLE course_versions (
  id BIGSERIAL PRIMARY KEY,
  course_slug TEXT NOT NULL REFERENCES courses(slug) ON DELETE CASCADE,
  version_number INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'published',
  source TEXT NOT NULL DEFAULT 'local_md',
  content_hash TEXT NOT NULL DEFAULT '',
  storage_prefix TEXT NOT NULL DEFAULT '',
  diff_from_previous TEXT NOT NULL DEFAULT '',
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (course_slug, version_number)
);

CREATE UNIQUE INDEX course_versions_published_hash_idx ON course_versions(course_slug, content_hash) WHERE status = 'published' AND content_hash <> '';
CREATE INDEX course_versions_latest_idx ON course_versions(course_slug, status, version_number DESC);

INSERT INTO course_versions (course_slug, version_number, status, source, content_hash, published_at)
SELECT slug, 1, 'published', 'local_md', '', now()
FROM courses
ON CONFLICT (course_slug, version_number) DO NOTHING;

ALTER TABLE course_attempts ADD COLUMN course_version_id BIGINT REFERENCES course_versions(id) ON DELETE RESTRICT;

UPDATE course_attempts ca
SET course_version_id = cv.id
FROM course_versions cv
WHERE cv.course_slug = ca.course_slug
  AND cv.version_number = 1;

ALTER TABLE course_attempts ALTER COLUMN course_version_id SET NOT NULL;
CREATE INDEX course_attempts_version_idx ON course_attempts(course_version_id);
