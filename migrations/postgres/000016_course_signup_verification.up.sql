-- Keep enrollment details with the email token so verification can finish on
-- another browser without creating or changing an account before verification.
ALTER TABLE student_login_tokens
  ADD COLUMN course_slug TEXT NOT NULL DEFAULT '',
  ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
