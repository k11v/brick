BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS builds (
    id uuid DEFAULT uuid_generate_v4()
);

COMMIT;
