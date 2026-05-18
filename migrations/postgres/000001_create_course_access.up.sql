-- +goose Up
CREATE TABLE people (
  id BIGSERIAL PRIMARY KEY,
  display_name TEXT NOT NULL DEFAULT '',
  avatar_url TEXT NOT NULL DEFAULT '',
  x_url TEXT NOT NULL DEFAULT '',
  instagram_url TEXT NOT NULL DEFAULT '',
  linkedin_url TEXT NOT NULL DEFAULT '',
  github_url TEXT NOT NULL DEFAULT '',
  nostr_npub TEXT NOT NULL DEFAULT '',
  timezone TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE person_emails (
  id BIGSERIAL PRIMARY KEY,
  person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  email TEXT NOT NULL,
  is_primary BOOLEAN NOT NULL DEFAULT false,
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (email)
);

CREATE INDEX person_emails_person_id_idx ON person_emails(person_id);

CREATE TABLE courses (
  slug TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE course_entitlements (
  id BIGSERIAL PRIMARY KEY,
  person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  course_slug TEXT NOT NULL REFERENCES courses(slug) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'active',
  source TEXT NOT NULL DEFAULT 'manual',
  external_source_id TEXT NOT NULL DEFAULT '',
  starts_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at TIMESTAMPTZ,
  notes TEXT NOT NULL DEFAULT '',
  UNIQUE (person_id, course_slug, source, external_source_id)
);

CREATE INDEX course_entitlements_lookup_idx ON course_entitlements(person_id, course_slug, status);

CREATE TABLE course_progress (
  id BIGSERIAL PRIMARY KEY,
  person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  course_slug TEXT NOT NULL REFERENCES courses(slug) ON DELETE CASCADE,
  lesson_path TEXT NOT NULL,
  block_id TEXT NOT NULL,
  block_type TEXT NOT NULL,
  correct BOOLEAN NOT NULL DEFAULT false,
  selected_option TEXT,
  selected_options_json TEXT NOT NULL DEFAULT '[]',
  answered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (person_id, course_slug, lesson_path, block_id)
);

CREATE INDEX course_progress_lesson_idx ON course_progress(person_id, course_slug, lesson_path);
