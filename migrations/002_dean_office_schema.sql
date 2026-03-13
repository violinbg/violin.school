-- Dean Office persisted chat conversations.
CREATE TABLE IF NOT EXISTS dean_conversations (
    id              TEXT     PRIMARY KEY,
    user_id         TEXT     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title           TEXT     NOT NULL DEFAULT '',
    summary         TEXT     NOT NULL DEFAULT '',
    status          TEXT     NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'archived')),
    last_message_at DATETIME,
    created_by      TEXT     NOT NULL DEFAULT 'dean.taskford',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by      TEXT     NOT NULL DEFAULT 'dean.taskford',
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_dean_conversations_user_updated_at
    ON dean_conversations (user_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_dean_conversations_user_created_at
    ON dean_conversations (user_id, created_at);

-- Dean Office messages within conversations.
CREATE TABLE IF NOT EXISTS dean_messages (
    id                  TEXT     PRIMARY KEY,
    conversation_id     TEXT     NOT NULL REFERENCES dean_conversations(id) ON DELETE CASCADE,
    user_id             TEXT     REFERENCES users(id) ON DELETE SET NULL,
    sequence            INTEGER  NOT NULL CHECK(sequence >= 0),
    role                TEXT     NOT NULL CHECK(role IN ('system', 'user', 'assistant', 'tool')),
    status              TEXT     NOT NULL DEFAULT 'completed' CHECK(status IN ('pending', 'streaming', 'completed', 'error', 'cancelled')),
    provider            TEXT     NOT NULL DEFAULT '',
    model               TEXT     NOT NULL DEFAULT '',
    content             TEXT     NOT NULL DEFAULT '',
    finish_reason       TEXT     NOT NULL DEFAULT '',
    provider_request_id TEXT     NOT NULL DEFAULT '',
    error_text          TEXT     NOT NULL DEFAULT '',
    created_by          TEXT     NOT NULL DEFAULT 'dean.taskford',
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by          TEXT     NOT NULL DEFAULT 'dean.taskford',
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (conversation_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_dean_messages_conversation_created_at
    ON dean_messages (conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_dean_messages_conversation_status_created_at
    ON dean_messages (conversation_id, status, created_at);

-- Extensible tool-calling records associated with assistant messages.
CREATE TABLE IF NOT EXISTS dean_tool_calls (
    id              TEXT     PRIMARY KEY,
    conversation_id TEXT     NOT NULL REFERENCES dean_conversations(id) ON DELETE CASCADE,
    message_id      TEXT     NOT NULL REFERENCES dean_messages(id) ON DELETE CASCADE,
    tool_call_id    TEXT     NOT NULL DEFAULT '',
    tool_name       TEXT     NOT NULL,
    status          TEXT     NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'running', 'completed', 'error', 'cancelled')),
    arguments_json  TEXT     NOT NULL DEFAULT '{}',
    result_json     TEXT     NOT NULL DEFAULT '',
    error_text      TEXT     NOT NULL DEFAULT '',
    started_at      DATETIME,
    completed_at    DATETIME,
    created_by      TEXT     NOT NULL DEFAULT 'dean.taskford',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by      TEXT     NOT NULL DEFAULT 'dean.taskford',
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_dean_tool_calls_conversation_created_at
    ON dean_tool_calls (conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_dean_tool_calls_message_created_at
    ON dean_tool_calls (message_id, created_at);
CREATE INDEX IF NOT EXISTS idx_dean_tool_calls_status_created_at
    ON dean_tool_calls (status, created_at);
