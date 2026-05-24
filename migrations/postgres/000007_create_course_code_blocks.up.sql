-- +goose Up
CREATE TABLE course_code_blocks (
  id BIGSERIAL PRIMARY KEY,
  person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  course_slug TEXT NOT NULL REFERENCES courses(slug) ON DELETE CASCADE,
  attempt_id BIGINT NOT NULL REFERENCES course_attempts(id) ON DELETE CASCADE,
  lesson_path TEXT NOT NULL,
  block_id TEXT NOT NULL,
  block_type TEXT NOT NULL,
  code_text TEXT NOT NULL DEFAULT '',
  output_text TEXT NOT NULL DEFAULT '',
  output_ok BOOLEAN,
  execution_count INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (attempt_id, lesson_path, block_id)
);

CREATE INDEX course_code_blocks_attempt_idx ON course_code_blocks(attempt_id);
CREATE INDEX course_code_blocks_lesson_idx ON course_code_blocks(person_id, course_slug, lesson_path);
