BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS operations (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    idempotency_key uuid NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    output_pdf_file_id uuid NOT NULL, -- TODO: NULL?
    process_log_file_id uuid NOT NULL, -- TODO: NULL?
    process_exit_code integer,
    PRIMARY KEY (id),
    FOREIGN KEY (output_pdf_file_id) REFERENCES operation_files (id),
    FOREIGN KEY (process_log_file_id) REFERENCES operation_files (id)
);
CREATE UNIQUE INDEX operations_idempotency_key_idx ON operations (idempotency_key);

CREATE TABLE IF NOT EXISTS operation_files (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    operation_id uuid NOT NULL,
    type text NOT NULL,
    name text NOT NULL,
    content_key text NOT NULL, -- TODO: NULL?
    PRIMARY KEY (id),
    FOREIGN KEY (operation_id) REFERENCES operations (id),
    CHECK (type IN ('input', 'output', 'run'))
);

CREATE TABLE IF NOT EXISTS user_locks (
    user_id uuid NOT NULL,
    PRIMARY KEY (user_id)
);

COMMIT;
