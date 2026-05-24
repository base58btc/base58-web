CREATE TABLE student_login_tokens (
  token TEXT PRIMARY KEY,
  email TEXT NOT NULL,
  next_path TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX student_login_tokens_email_idx ON student_login_tokens(email);
CREATE INDEX student_login_tokens_expires_idx ON student_login_tokens(expires_at);
