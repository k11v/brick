BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS builds (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    idempotency_key uuid NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    output_file_key text, -- NULL when inserted, then updated to be non-NULL
    log_file_key text, -- NULL when inserted, then updated to be non-NULL
    exit_code integer,
    status text NOT NULL,
    PRIMARY KEY (id)
);
CREATE UNIQUE INDEX builds_idempotency_key_idx ON builds (idempotency_key);

CREATE TABLE IF NOT EXISTS build_input_files (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    build_id uuid NOT NULL,
    name text NOT NULL,
    content_key text, -- NULL when inserted, then updated to be non-NULL
    PRIMARY KEY (id),
    FOREIGN KEY (build_id) REFERENCES builds (id)
);

CREATE TABLE IF NOT EXISTS user_locks (
    user_id uuid NOT NULL,
    PRIMARY KEY (user_id)
);

CREATE TABLE IF NOT EXISTS revoked_tokens (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    expires_at timestamp with time zone NOT NULL,
    PRIMARY KEY (id)
);

COMMIT;
