ALTER TABLE course_page_views DROP CONSTRAINT IF EXISTS course_page_views_attempt_lesson_key;
ALTER TABLE course_progress DROP CONSTRAINT IF EXISTS course_progress_attempt_lesson_block_key;

DROP INDEX IF EXISTS course_page_views_attempt_idx;
DROP INDEX IF EXISTS course_progress_attempt_idx;

ALTER TABLE course_page_views DROP COLUMN IF EXISTS attempt_id;
ALTER TABLE course_progress DROP COLUMN IF EXISTS attempt_id;

ALTER TABLE course_page_views ADD CONSTRAINT course_page_views_person_id_course_slug_lesson_path_key UNIQUE (person_id, course_slug, lesson_path);
ALTER TABLE course_progress ADD CONSTRAINT course_progress_person_id_course_slug_lesson_path_block_id_key UNIQUE (person_id, course_slug, lesson_path, block_id);

DROP INDEX IF EXISTS course_attempts_person_course_idx;
DROP INDEX IF EXISTS course_attempts_active_idx;
DROP TABLE IF EXISTS course_attempts;
