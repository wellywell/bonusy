BEGIN;

CREATE TABLE balance (id BIGSERIAL PRIMARY KEY, user_id BIGINT, current DOUBLE PRECISION, withdrawn DOUBLE PRECISION,
    CONSTRAINT fk_user_id
    FOREIGN KEY(user_id) 
    REFERENCES auth_user(id)
    ON DELETE NO ACTION);

CREATE UNIQUE INDEX user_id_balance_idx ON balance(user_id);

COMMIT;