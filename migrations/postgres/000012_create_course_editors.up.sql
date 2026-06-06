CREATE TABLE course_editors (
  id BIGSERIAL PRIMARY KEY,
  course_slug TEXT NOT NULL REFERENCES courses(slug) ON DELETE CASCADE,
  person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'editor',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (course_slug, person_id)
);

CREATE INDEX course_editors_lookup_idx ON course_editors(person_id, course_slug, status);
