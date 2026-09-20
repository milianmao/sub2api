DO $$
BEGIN
  ALTER TABLE groups
    DROP CONSTRAINT IF EXISTS groups_openai_image_upstream_check;

  ALTER TABLE groups
    ADD CONSTRAINT groups_openai_image_upstream_check
    CHECK (openai_image_upstream IN ('auto', 'official_images', 'codex_responses', 'codex_images', 'chatgpt_web_image'));
END $$;
