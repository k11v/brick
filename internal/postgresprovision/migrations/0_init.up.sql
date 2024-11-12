BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS builds (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    idempotency_key uuid NOT NULL,

    user_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),

    -- document_token text, -- instead of document_cache_files jsonb
    -- document_files jsonb,
    document_token text,

    -- process_log_file text,
    process_log_token text,
    process_used_time interval,
    process_used_memory integer,
    process_exit_code integer,

    -- output_file text,
    output_token text,
    next_document_token text, -- instead of output_cache_files jsonb
    output_expires_at timestamp with time zone,

    status text NOT NULL,

    PRIMARY KEY (id),
    CHECK (status IN ('pending', 'running', 'completed', 'canceled'))
);

COMMIT;
