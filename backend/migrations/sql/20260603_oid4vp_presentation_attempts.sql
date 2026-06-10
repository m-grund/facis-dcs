CREATE TYPE oid4vp_presentation_status AS ENUM (
  'pending',
  'complete',
  'failed',
  'expired'
);

CREATE TABLE oid4vp_presentation_attempts (
  presentation_state TEXT PRIMARY KEY,
  status oid4vp_presentation_status NOT NULL,
  nonce TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  verified_claims JSONB,
  hydra_login_challenge TEXT,
  redirect_uri TEXT,
  error_message TEXT,
  subject_did TEXT,
  organization_id TEXT,
  roles JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX oid4vp_presentation_attempts_expires_at_idx
  ON oid4vp_presentation_attempts (expires_at);
