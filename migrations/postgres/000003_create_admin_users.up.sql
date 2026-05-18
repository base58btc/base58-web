CREATE TABLE admin_users (
  id BIGSERIAL PRIMARY KEY,
  person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'admin',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (person_id)
);

CREATE INDEX admin_users_role_idx ON admin_users(role, status);

CREATE TABLE admin_login_tokens (
  token TEXT PRIMARY KEY,
  email TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX admin_login_tokens_email_idx ON admin_login_tokens(email);
