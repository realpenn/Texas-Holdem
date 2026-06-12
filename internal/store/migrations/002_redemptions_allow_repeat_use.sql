CREATE TABLE IF NOT EXISTS redemptions_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL,
    chat_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    amount INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO redemptions_new(id, code, chat_id, user_id, amount, created_at)
SELECT id, code, chat_id, user_id, amount, created_at
FROM redemptions;

DROP TABLE redemptions;

ALTER TABLE redemptions_new RENAME TO redemptions;

CREATE INDEX IF NOT EXISTS idx_redemptions_code ON redemptions(code);
CREATE INDEX IF NOT EXISTS idx_redemptions_chat_user ON redemptions(chat_id, user_id);
