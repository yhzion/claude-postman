CREATE TABLE sessions (
    id              TEXT PRIMARY KEY,
    tmux_name       TEXT NOT NULL,
    working_dir     TEXT NOT NULL,
    model           TEXT NOT NULL,
    status          TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_prompt     TEXT,
    last_result     TEXT
);

CREATE TABLE outbox (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    message_id      TEXT,
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    attachments     TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    next_retry_at   DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sent_at         DATETIME,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE TABLE inbox (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    body            TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed       INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE TABLE template (
    id              TEXT PRIMARY KEY,
    message_id      TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sessions_status ON sessions(status);
CREATE INDEX idx_outbox_status ON outbox(status);
CREATE INDEX idx_inbox_session_processed ON inbox(session_id, processed);

CREATE TABLE schema_version (
    version         INTEGER NOT NULL
);

INSERT INTO schema_version (version) VALUES (1);
