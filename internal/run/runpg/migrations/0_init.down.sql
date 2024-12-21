BEGIN;

DROP TABLE IF EXISTS revoked_access_tokens;

DROP TABLE IF EXISTS user_locks;

DROP TABLE IF EXISTS operation_input_files;

DROP TABLE IF EXISTS operations;

DROP EXTENSION IF EXISTS "uuid-ossp";

COMMIT;
