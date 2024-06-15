BEGIN;

CREATE TABLE user_order (id BIGSERIAL PRIMARY KEY, order_number VARCHAR(255), user_id BIGINT, accrual DOUBLE PRECISION, status VARCHAR(20), uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT fk_user_id
    FOREIGN KEY(user_id) 
    REFERENCES auth_user(id)
    ON DELETE NO ACTION);

CREATE UNIQUE INDEX number_idx ON user_order(order_number);
CREATE INDEX user_id_idx ON user_order(user_id);

COMMIT;