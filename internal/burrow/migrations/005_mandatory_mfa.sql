ALTER TABLE users ADD COLUMN mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN mfa_cipher TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN mfa_last_step BIGINT NOT NULL DEFAULT -1;
ALTER TABLE users ADD COLUMN auth_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN mfa_at TIMESTAMP;
ALTER TABLE sessions ADD COLUMN auth_version BIGINT NOT NULL DEFAULT 0;
CREATE TABLE login_transactions (
    id TEXT PRIMARY KEY,
    credential_hash TEXT NOT NULL UNIQUE,
    browser_hash TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    auth_version BIGINT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    pending_cipher TEXT NOT NULL DEFAULT '',
    mfa_verified BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_at TIMESTAMP,
    attempts INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_login_transactions_user_id ON login_transactions(user_id);
CREATE INDEX idx_login_transactions_expires_at ON login_transactions(expires_at);
UPDATE sessions SET revoked = TRUE;
UPDATE token_records SET revoked = TRUE;
DELETE FROM auth_transactions;
INSERT INTO events (id, actor_id, object_id, kind, success, request_id, details, created_at)
VALUES ('migration:005:mandatory_mfa', '', '', 'authentication:migrate', TRUE, '', '{"policy":"mandatory_mfa","existingSessionsRevoked":true}', CURRENT_TIMESTAMP)
ON CONFLICT (id) DO NOTHING;
