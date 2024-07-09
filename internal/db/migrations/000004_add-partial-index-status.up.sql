BEGIN;

CREATE index order_status_partial_idx on user_order(id, status) 
WHERE user_order.status not in ('INVALID', 'PROCESSED');

COMMIT;