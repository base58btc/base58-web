-- Historical subscribers and messages remain in Notion. Retain subsequent
-- opt-outs here so a later import cannot accidentally resubscribe someone.
CREATE TABLE IF NOT EXISTS email_opt_outs (
  email TEXT NOT NULL,
  newsletter TEXT NOT NULL,
  original_email TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  mailer_cancelled_at TIMESTAMPTZ,
  PRIMARY KEY (email, newsletter)
);
