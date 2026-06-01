ALTER TABLE card_mailbox_credentials
    ADD COLUMN IF NOT EXISTS raw_json text;

UPDATE card_mailbox_credentials
SET raw_json = json_build_object(
    'email', email,
    'mailbox_url', mailbox_url
)::text
WHERE raw_json IS NULL OR btrim(raw_json) = '';

ALTER TABLE card_mailbox_credentials
    ALTER COLUMN raw_json SET NOT NULL;
