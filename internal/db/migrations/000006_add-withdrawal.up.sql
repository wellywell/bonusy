BEGIN;

CREATE TABLE withdrawal (id BIGSERIAL PRIMARY KEY, user_id BIGINT, sum DOUBLE PRECISION NOT NULL, order_name VARCHAR(255) NOT NULL, processed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT fk_user_id
    FOREIGN KEY(user_id) 
    REFERENCES auth_user(id)
    ON DELETE NO ACTION);

CREATE INDEX user_id_withdrawal_idx ON balance(user_id);

COMMIT;