CREATE TABLE course_page_views (
  id BIGSERIAL PRIMARY KEY,
  person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  course_slug TEXT NOT NULL REFERENCES courses(slug) ON DELETE CASCADE,
  lesson_path TEXT NOT NULL,
  first_viewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_viewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  view_count INTEGER NOT NULL DEFAULT 1,
  UNIQUE (person_id, course_slug, lesson_path)
);

CREATE INDEX course_page_views_person_course_idx ON course_page_views(person_id, course_slug);
