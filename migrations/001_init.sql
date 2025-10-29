-- 001_init.sql
-- Create base profiles table and indices
CREATE TABLE IF NOT EXISTS profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name STRING NOT NULL,
    location_country STRING NOT NULL,
    location_city STRING NOT NULL,
    description STRING(160) NOT NULL,
    photo_webp BYTES NOT NULL,
    photo_content_type STRING NOT NULL DEFAULT 'image/webp',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    votes_count INT NOT NULL DEFAULT 0,
    search_text STRING NOT NULL AS (lower(full_name || ' ' || location_country || ' ' || location_city || ' ' || description)) STORED
);

CREATE INDEX IF NOT EXISTS idx_profiles_sort ON profiles (votes_count DESC, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_profiles_search ON profiles (search_text);
