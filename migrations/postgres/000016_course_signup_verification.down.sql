-- Invalidate pending course links before removing their enrollment context.
DELETE FROM student_login_tokens WHERE course_slug <> '';
ALTER TABLE student_login_tokens
  DROP COLUMN display_name,
  DROP COLUMN course_slug;
