BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- -- TODO
-- CREATE TABLE IF NOT EXISTS users ();

CREATE TABLE IF NOT EXISTS builds (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),

    idempotency_key uuid NOT NULL,

    user_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),

    document_files jsonb,
    document_cache_files jsonb,

    process_log_file text,
    process_used_time interval,
    process_used_memory integer,
    process_exit_code integer,

    result_pdf_file text,
    result_cache_files jsonb,
    result_expires_at timestamp with time zone,

    status text NOT NULL,

    PRIMARY KEY (id),
    CHECK (status IN ('pending', 'running', 'completed', 'canceled'))
);

COMMIT;
