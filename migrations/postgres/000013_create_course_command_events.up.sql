CREATE TABLE course_command_events (
  attempt_id BIGINT NOT NULL REFERENCES course_attempts(id) ON DELETE CASCADE,
  environment_hash TEXT NOT NULL,
  revision BIGINT NOT NULL,
  event_id TEXT NOT NULL,
  event_json TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (attempt_id, environment_hash, revision),
  UNIQUE (attempt_id, environment_hash, event_id)
);

CREATE TABLE course_vm_sessions (
  attempt_id BIGINT NOT NULL REFERENCES course_attempts(id) ON DELETE CASCADE,
  environment_hash TEXT NOT NULL,
  owner TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (attempt_id, environment_hash)
);

-- Compressed, content-addressed pieces are private to a course attempt. The
-- transport can later use object storage without changing journal semantics.
CREATE TABLE course_vm_chunks (
  attempt_id BIGINT NOT NULL REFERENCES course_attempts(id) ON DELETE CASCADE,
  environment_hash TEXT NOT NULL,
  hash TEXT NOT NULL,
  data BYTEA NOT NULL,
  PRIMARY KEY (attempt_id, environment_hash, hash)
);
CREATE TABLE course_vm_checkpoints (
  attempt_id BIGINT NOT NULL REFERENCES course_attempts(id) ON DELETE CASCADE,
  environment_hash TEXT NOT NULL,
  checkpoint_id TEXT NOT NULL,
  descriptor TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (attempt_id, environment_hash, checkpoint_id)
);
