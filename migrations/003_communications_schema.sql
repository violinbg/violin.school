-- Replace dean-specific messaging schema with generalized communication schema.
DROP TABLE IF EXISTS dean_tool_calls;
DROP TABLE IF EXISTS dean_messages;
DROP TABLE IF EXISTS dean_conversations;

CREATE TABLE IF NOT EXISTS communication_conversations (
    id              TEXT     PRIMARY KEY,
    user_id         TEXT     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_type    TEXT     NOT NULL CHECK(channel_type IN ('mail_thread', 'context_chat')),
    context_key     TEXT     NOT NULL DEFAULT '',
    recipient_kind  TEXT     NOT NULL DEFAULT 'dean_office',
    recipient_id    TEXT     NOT NULL DEFAULT 'dean.taskford',
    title           TEXT     NOT NULL DEFAULT '',
    summary         TEXT     NOT NULL DEFAULT '',
    status          TEXT     NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'archived')),
    last_message_at DATETIME,
    created_by      TEXT     NOT NULL DEFAULT 'dean.taskford',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by      TEXT     NOT NULL DEFAULT 'dean.taskford',
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_comm_conversations_user_channel_updated_at
    ON communication_conversations (user_id, channel_type, updated_at);
CREATE INDEX IF NOT EXISTS idx_comm_conversations_user_channel_created_at
    ON communication_conversations (user_id, channel_type, created_at);
CREATE INDEX IF NOT EXISTS idx_comm_conversations_context
    ON communication_conversations (user_id, channel_type, context_key, created_at);
CREATE INDEX IF NOT EXISTS idx_comm_conversations_recipient
    ON communication_conversations (user_id, recipient_kind, recipient_id, created_at);

CREATE TABLE IF NOT EXISTS communication_messages (
    id                  TEXT     PRIMARY KEY,
    conversation_id     TEXT     NOT NULL REFERENCES communication_conversations(id) ON DELETE CASCADE,
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

CREATE INDEX IF NOT EXISTS idx_comm_messages_conversation_created_at
    ON communication_messages (conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_comm_messages_conversation_status_created_at
    ON communication_messages (conversation_id, status, created_at);

CREATE TABLE IF NOT EXISTS communication_tool_calls (
    id              TEXT     PRIMARY KEY,
    conversation_id TEXT     NOT NULL REFERENCES communication_conversations(id) ON DELETE CASCADE,
    message_id      TEXT     NOT NULL REFERENCES communication_messages(id) ON DELETE CASCADE,
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

CREATE INDEX IF NOT EXISTS idx_comm_tool_calls_conversation_created_at
    ON communication_tool_calls (conversation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_comm_tool_calls_message_created_at
    ON communication_tool_calls (message_id, created_at);
CREATE INDEX IF NOT EXISTS idx_comm_tool_calls_status_created_at
    ON communication_tool_calls (status, created_at);
