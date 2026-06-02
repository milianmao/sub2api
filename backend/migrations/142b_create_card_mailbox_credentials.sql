CREATE TABLE IF NOT EXISTS card_mailbox_credentials (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    email VARCHAR(255) NOT NULL,
    mailbox_url TEXT NOT NULL,
    last_code VARCHAR(64),
    last_status VARCHAR(20),
    last_error TEXT,
    last_fetched_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS cardmailboxcredential_email
    ON card_mailbox_credentials (email);

CREATE INDEX IF NOT EXISTS cardmailboxcredential_last_status
    ON card_mailbox_credentials (last_status);

CREATE INDEX IF NOT EXISTS cardmailboxcredential_last_fetched_at
    ON card_mailbox_credentials (last_fetched_at);
