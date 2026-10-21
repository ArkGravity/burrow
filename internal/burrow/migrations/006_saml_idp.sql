ALTER TABLE applications ADD COLUMN protocol TEXT NOT NULL DEFAULT 'oidc' CHECK (protocol IN ('oidc', 'saml'));
ALTER TABLE login_transactions ADD COLUMN request_type TEXT NOT NULL DEFAULT 'oidc' CHECK (request_type IN ('oidc', 'saml'));
ALTER TABLE sessions ADD COLUMN utc_timestamps BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE saml_applications (
    application_id TEXT PRIMARY KEY REFERENCES applications(id) ON DELETE CASCADE,
    entity_id TEXT NOT NULL UNIQUE,
    acs_url TEXT NOT NULL,
    signing_certificate TEXT NOT NULL,
    metadata_xml TEXT NOT NULL,
    config_version BIGINT NOT NULL DEFAULT 1 CHECK (config_version > 0)
);
CREATE TABLE saml_transactions (
    id TEXT PRIMARY KEY,
    application_id TEXT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    sp_request_id TEXT NOT NULL,
    browser_hash TEXT NOT NULL,
    relay_state TEXT NOT NULL DEFAULT '',
    acs_url TEXT NOT NULL,
    config_version BIGINT NOT NULL,
    force_authn BOOLEAN NOT NULL DEFAULT FALSE,
    failure_status TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    consumed BOOLEAN NOT NULL DEFAULT FALSE,
    user_id TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX saml_transactions_request ON saml_transactions(application_id, sp_request_id);
CREATE INDEX saml_transactions_expiry ON saml_transactions(expires_at);
CREATE INDEX saml_transactions_user ON saml_transactions(user_id);

CREATE TABLE saml_request_replays (
    sp_entity_id TEXT NOT NULL,
    sp_request_id TEXT NOT NULL,
    replay_until TIMESTAMP NOT NULL,
    PRIMARY KEY (sp_entity_id, sp_request_id)
);
CREATE INDEX saml_request_replays_expiry ON saml_request_replays(replay_until);

CREATE TABLE saml_signing_keys (
    id TEXT PRIMARY KEY,
    private_cipher TEXT NOT NULL,
    certificate_pem TEXT NOT NULL,
    active BOOLEAN NOT NULL,
    created_at TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX saml_signing_keys_active ON saml_signing_keys(active) WHERE active = TRUE;
