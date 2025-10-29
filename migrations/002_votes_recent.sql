-- 002_votes_recent.sql
-- Add votes_recent table to enforce per-profile 60-minute vote window
CREATE TABLE IF NOT EXISTS votes_recent (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_votes_recent_profile_created ON votes_recent (profile_id, created_at DESC);
