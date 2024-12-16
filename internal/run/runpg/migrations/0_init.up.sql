BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS operations (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    idempotency_key uuid NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    output_file_key text, -- NULL when inserted, then updated to be non-NULL
    log_file_key text, -- NULL when inserted, then updated to be non-NULL
    exit_code integer,
    PRIMARY KEY (id)
);
CREATE UNIQUE INDEX operations_idempotency_key_idx ON operations (idempotency_key);

CREATE TABLE IF NOT EXISTS operation_input_files (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    operation_id uuid NOT NULL,
    name text NOT NULL,
    content_key text, -- NULL when inserted, then updated to be non-NULL
    PRIMARY KEY (id),
    FOREIGN KEY (operation_id) REFERENCES operations (id)
);

CREATE TABLE IF NOT EXISTS user_locks (
    user_id uuid NOT NULL,
    PRIMARY KEY (user_id)
);

COMMIT;
