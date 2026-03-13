-- Violin School — fresh baseline schema.
-- Domain-table audit rule:
-- - Required on domain tables: created_by, created_at, updated_by, updated_at
-- - active is included only where soft-disable is needed.

-- Application users.
CREATE TABLE IF NOT EXISTS users (
    id            TEXT     PRIMARY KEY,
    username      TEXT     UNIQUE NOT NULL,
    full_name     TEXT     NOT NULL,
    password_hash TEXT     NOT NULL,
    role          TEXT     NOT NULL DEFAULT 'user',
    active        INTEGER  NOT NULL DEFAULT 1,
    language      TEXT     NOT NULL DEFAULT 'en',
    last_login    DATETIME,
    created_by    TEXT     NOT NULL DEFAULT 'dean.taskford',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by    TEXT     NOT NULL DEFAULT 'dean.taskford',
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed system account for non-user AI cost attribution.
INSERT OR IGNORE INTO users (
    id, username, full_name, password_hash, role, active, language, created_by, updated_by
) VALUES (
    'dean.taskford',
    'dean.taskford',
    'Dean Augustus Taskford',
    'system-account-no-login',
    'system',
    0,
    'en',
    'dean.taskford',
    'dean.taskford'
);

-- Key/value application configuration.
CREATE TABLE IF NOT EXISTS app_config (
    key        TEXT     PRIMARY KEY,
    value      TEXT     NOT NULL,
    created_by TEXT     NOT NULL DEFAULT 'dean.taskford',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by TEXT     NOT NULL DEFAULT 'dean.taskford',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed default configuration values.
INSERT OR IGNORE INTO app_config (key, value) VALUES ('registration_enabled', 'true');
INSERT OR IGNORE INTO app_config (key, value) VALUES ('max_users', '100');
INSERT OR IGNORE INTO app_config (key, value) VALUES ('xai_api_token_ciphertext', '');
INSERT OR IGNORE INTO app_config (key, value) VALUES ('xai_base_url', 'https://api.x.ai/v1');
INSERT OR IGNORE INTO app_config (key, value) VALUES ('xai_monthly_global_limit_usd_ticks', '1000000000000');
INSERT OR IGNORE INTO app_config (key, value) VALUES ('xai_monthly_default_user_limit_usd_ticks', '250000000000');

-- JWT refresh tokens for session management.
CREATE TABLE IF NOT EXISTS refresh_tokens (
    token_hash  TEXT     PRIMARY KEY,
    family      TEXT     NOT NULL,
    user_id     TEXT     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  DATETIME NOT NULL,
    used        INTEGER  NOT NULL DEFAULT 0,
    created_by  TEXT     NOT NULL DEFAULT 'dean.taskford',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by  TEXT     NOT NULL DEFAULT 'dean.taskford',
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens (family);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires ON refresh_tokens (expires_at);

-- Courses — core learning platform content.
CREATE TABLE IF NOT EXISTS courses (
    id          TEXT     PRIMARY KEY,
    title       TEXT     NOT NULL,
    description TEXT     NOT NULL DEFAULT '',
    author_id   TEXT     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    published   INTEGER  NOT NULL DEFAULT 0,
    active      INTEGER  NOT NULL DEFAULT 1,
    created_by  TEXT     NOT NULL DEFAULT 'dean.taskford',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by  TEXT     NOT NULL DEFAULT 'dean.taskford',
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_courses_published ON courses (published);
CREATE INDEX IF NOT EXISTS idx_courses_author ON courses (author_id);

-- xAI usage tracking events.
CREATE TABLE IF NOT EXISTS ai_usage_events (
    id                TEXT PRIMARY KEY,
    user_id           TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider          TEXT    NOT NULL,
    model             TEXT    NOT NULL,
    request_scope     TEXT    NOT NULL,
    total_tokens      INTEGER NOT NULL DEFAULT 0 CHECK(total_tokens >= 0),
    cost_in_usd_ticks INTEGER NOT NULL DEFAULT 0 CHECK(cost_in_usd_ticks >= 0),
    request_id        TEXT    NOT NULL DEFAULT '',
    created_by        TEXT    NOT NULL DEFAULT 'dean.taskford',
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by        TEXT    NOT NULL DEFAULT 'dean.taskford',
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ai_usage_events_created_at ON ai_usage_events (created_at);
CREATE INDEX IF NOT EXISTS idx_ai_usage_events_user_month ON ai_usage_events (user_id, created_at);

-- Optional per-user monthly limit overrides.
CREATE TABLE IF NOT EXISTS user_ai_limits (
    user_id                 TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    monthly_limit_usd_ticks INTEGER NOT NULL CHECK(monthly_limit_usd_ticks >= 0),
    created_by              TEXT NOT NULL DEFAULT 'dean.taskford',
    created_at              DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by              TEXT NOT NULL DEFAULT 'dean.taskford',
    updated_at              DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
