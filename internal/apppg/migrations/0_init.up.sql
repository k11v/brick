BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS builds (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    idempotency_key uuid NOT NULL,
    user_id uuid NOT NULL,
    
    status text NOT NULL,
    error text,
    exit_code int,
    log_data_key text NOT NULL,
    output_data_key text NOT NULL,

    PRIMARY KEY (id)
);
CREATE UNIQUE INDEX builds_idempotency_key_idx ON builds (idempotency_key);

CREATE TABLE IF NOT EXISTS build_files (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    build_id uuid NOT NULL,

    name text NOT NULL,
    type text NOT NULL,
    data_key text NOT NULL,

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
