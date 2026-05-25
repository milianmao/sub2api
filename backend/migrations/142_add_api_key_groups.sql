CREATE TABLE IF NOT EXISTS api_key_groups (
    api_key_id bigint NOT NULL,
    group_id bigint NOT NULL,
    priority integer NOT NULL DEFAULT 50,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    PRIMARY KEY (api_key_id, group_id),
    CONSTRAINT api_key_groups_api_key_id_fkey
        FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE,
    CONSTRAINT api_key_groups_group_id_fkey
        FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_api_key_groups_group_id ON api_key_groups(group_id);
CREATE INDEX IF NOT EXISTS idx_api_key_groups_priority ON api_key_groups(priority);
CREATE INDEX IF NOT EXISTS idx_api_key_groups_api_key_id_priority ON api_key_groups(api_key_id, priority, group_id);

INSERT INTO api_key_groups (api_key_id, group_id, priority, created_at)
SELECT id, group_id, 50, NOW()
FROM api_keys
WHERE group_id IS NOT NULL
ON CONFLICT (api_key_id, group_id) DO NOTHING;
