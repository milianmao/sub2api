ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS openai_image_upstream TEXT NOT NULL DEFAULT 'auto';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'groups_openai_image_upstream_check'
  ) THEN
    ALTER TABLE groups
      ADD CONSTRAINT groups_openai_image_upstream_check
      CHECK (openai_image_upstream IN ('auto', 'official_images', 'codex_responses', 'chatgpt_web_image'));
  END IF;
END $$;
