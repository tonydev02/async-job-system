CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    status TEXT NOT NULL,
    payload JSONB NOT NULL,
    result JSONB,
    error TEXT,
    attempt INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CONSTRAINT jobs_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE INDEX jobs_status_created_at_idx ON jobs (status, created_at);
CREATE INDEX jobs_next_run_at_idx ON jobs (next_run_at);
