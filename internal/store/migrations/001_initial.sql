CREATE TABLE inbox_items (
    id                INTEGER PRIMARY KEY,
    source_type       TEXT NOT NULL,
    source_id         TEXT NOT NULL,
    kind              TEXT NOT NULL,
    number            INTEGER,
    title             TEXT NOT NULL DEFAULT '',
    body              TEXT NOT NULL DEFAULT '',
    author            TEXT NOT NULL DEFAULT '',
    labels            TEXT NOT NULL DEFAULT '[]',
    owner             TEXT NOT NULL DEFAULT '',
    repo              TEXT NOT NULL DEFAULT '',
    metadata          TEXT NOT NULL DEFAULT '{}',
    status            TEXT NOT NULL DEFAULT 'new',
    worktree_path     TEXT NOT NULL DEFAULT '',
    source_updated_at TIMESTAMP,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_inbox_items_source_id ON inbox_items(source_id);
CREATE INDEX idx_inbox_items_status ON inbox_items(status);

CREATE TABLE runs (
    id               INTEGER PRIMARY KEY,
    inbox_item_id    INTEGER NOT NULL REFERENCES inbox_items(id),
    workflow_type    TEXT NOT NULL,
    agent_name       TEXT NOT NULL DEFAULT '',
    agent_session_id TEXT NOT NULL DEFAULT '',
    worktree_path    TEXT NOT NULL DEFAULT '',
    resume_cmd       TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'running',
    output_file      TEXT NOT NULL DEFAULT '',
    started_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at     TIMESTAMP
);

CREATE INDEX idx_runs_inbox_item_id ON runs(inbox_item_id);
CREATE INDEX idx_runs_status ON runs(status);
