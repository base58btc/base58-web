CREATE TABLE course_attempts (
  id BIGSERIAL PRIMARY KEY,
  person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  course_slug TEXT NOT NULL REFERENCES courses(slug) ON DELETE CASCADE,
  attempt_number INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  reset_at TIMESTAMPTZ,
  reset_reason TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT 'student',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (person_id, course_slug, attempt_number)
);

CREATE UNIQUE INDEX course_attempts_active_idx ON course_attempts(person_id, course_slug) WHERE status = 'active';
CREATE INDEX course_attempts_person_course_idx ON course_attempts(person_id, course_slug, started_at DESC);

INSERT INTO course_attempts (person_id, course_slug, attempt_number, status, created_by)
SELECT DISTINCT person_id, course_slug, 1, 'active', 'system'
FROM course_entitlements
WHERE status = 'active'
ON CONFLICT (person_id, course_slug, attempt_number) DO NOTHING;

INSERT INTO course_attempts (person_id, course_slug, attempt_number, status, created_by)
SELECT DISTINCT person_id, course_slug, 1, 'active', 'system'
FROM course_progress
ON CONFLICT (person_id, course_slug, attempt_number) DO NOTHING;

INSERT INTO course_attempts (person_id, course_slug, attempt_number, status, created_by)
SELECT DISTINCT person_id, course_slug, 1, 'active', 'system'
FROM course_page_views
ON CONFLICT (person_id, course_slug, attempt_number) DO NOTHING;

ALTER TABLE course_progress ADD COLUMN attempt_id BIGINT REFERENCES course_attempts(id) ON DELETE CASCADE;
ALTER TABLE course_page_views ADD COLUMN attempt_id BIGINT REFERENCES course_attempts(id) ON DELETE CASCADE;

UPDATE course_progress cp
SET attempt_id = ca.id
FROM course_attempts ca
WHERE ca.person_id = cp.person_id
  AND ca.course_slug = cp.course_slug
  AND ca.attempt_number = 1;

UPDATE course_page_views cpv
SET attempt_id = ca.id
FROM course_attempts ca
WHERE ca.person_id = cpv.person_id
  AND ca.course_slug = cpv.course_slug
  AND ca.attempt_number = 1;

ALTER TABLE course_progress ALTER COLUMN attempt_id SET NOT NULL;
ALTER TABLE course_page_views ALTER COLUMN attempt_id SET NOT NULL;

ALTER TABLE course_progress DROP CONSTRAINT IF EXISTS course_progress_person_id_course_slug_lesson_path_block_id_key;
ALTER TABLE course_page_views DROP CONSTRAINT IF EXISTS course_page_views_person_id_course_slug_lesson_path_key;

ALTER TABLE course_progress ADD CONSTRAINT course_progress_attempt_lesson_block_key UNIQUE (attempt_id, lesson_path, block_id);
ALTER TABLE course_page_views ADD CONSTRAINT course_page_views_attempt_lesson_key UNIQUE (attempt_id, lesson_path);

CREATE INDEX course_progress_attempt_idx ON course_progress(attempt_id);
CREATE INDEX course_page_views_attempt_idx ON course_page_views(attempt_id);
