BEGIN;

CREATE UNIQUE INDEX username_index ON auth_user(username);

COMMIT;