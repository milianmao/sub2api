-- Add OpenAI compatibility opt-in for Anthropic/Antigravity groups.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allow_openai_compat BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE groups
SET allow_openai_compat = FALSE
WHERE platform NOT IN ('anthropic', 'antigravity')
  AND allow_openai_compat IS DISTINCT FROM FALSE;
