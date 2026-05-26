UPDATE api_keys k
SET group_id = NULL,
    updated_at = NOW()
WHERE k.group_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM groups g
      WHERE g.id = k.group_id
  );
