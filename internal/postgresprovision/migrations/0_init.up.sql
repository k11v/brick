BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- -- TODO
-- CREATE TABLE IF NOT EXISTS users ();

CREATE TABLE IF NOT EXISTS builds (
    id uuid NOT NULL DEFAULT uuid_generate_v4(),
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    idempotency_key uuid NOT NULL,
    status text NOT NULL,
    user_id uuid NOT NULL,
    PRIMARY KEY (id)
);

COMMIT;
