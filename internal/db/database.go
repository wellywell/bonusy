package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wellywell/bonusy/internal/types"
)

type Database struct {
	pool *pgxpool.Pool
}

func NewDatabase(connString string) (*Database, error) {

	err := Migrate(connString)

	if err != nil {
		return nil, fmt.Errorf("failed to migrate %w", err)
	}

	ctx := context.Background()
	p, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}

	return &Database{
		pool: p,
	}, nil
}

func (d *Database) CreateUser(ctx context.Context, username string, password string) error {

	query := `
		INSERT INTO auth_user (username, password)
		VALUES ($1, $2)
		`
	_, err := d.pool.Exec(ctx, query, username, password)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgerrcode.IsIntegrityConstraintViolation(pgErr.Code) {
			return fmt.Errorf("%w", &UserExistsError{Username: username})
		}
		return err
	}
	return nil
}

func (d *Database) GetUserHashedPassword(ctx context.Context, username string) (string, error) {
	query := `
		SELECT password 
		FROM auth_user 
		WHERE username = $1`

	row := d.pool.QueryRow(ctx, query, username)

	var password string

	err := row.Scan(&password)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w", &UserNotFoundError{Username: username})
	}
	return password, nil
}

func (d *Database) GetUserID(ctx context.Context, username string) (int, error) {
	query := `
		SELECT id 
		FROM auth_user 
		WHERE username = $1`

	row := d.pool.QueryRow(ctx, query, username)

	var id int

	err := row.Scan(&id)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w", &UserNotFoundError{Username: username})
	}
	return id, nil

}

func (d *Database) InsertUserOrder(ctx context.Context, order string, userID int, status types.Status) error {

	query := `
	WITH inserted AS
		(INSERT INTO user_order (user_id, order_number, status)
		 VALUES ($1, $2, $3)
		 ON CONFLICT(order_number) DO NOTHING
		 RETURNING -1)
	SELECT COALESCE (
		(SELECT * FROM inserted),
		(SELECT user_id FROM user_order WHERE order_number = $2)
	)`

	row := d.pool.QueryRow(ctx, query, userID, order, status)

	var OwnerOfExistingRow int
	if err := row.Scan(&OwnerOfExistingRow); err != nil {
		return fmt.Errorf("%w", err)
	}

	// row was inserted sucessfully
	if OwnerOfExistingRow == -1 {
		return nil
	}

	if OwnerOfExistingRow == userID {
		return fmt.Errorf("%w", &UserAlreadyUploadedOrder{userID, order})
	} else {
		return fmt.Errorf("%w", &OrderUploadedByWrongUser{order})
	}
}

func (d *Database) GetUnprocessedOrders(ctx context.Context, startID int, limit int) ([]types.OrderRecord, error) {
	query := `
	    SELECT id, order_number, status
		FROM user_order
		WHERE status not in ('INVALID', 'PROCESSED')
		AND id > $1
		ORDER BY id LIMIT $2
	`
	rows, err := d.pool.Query(ctx, query, startID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed collecting rows %w", err)
	}

	orders, err := pgx.CollectRows(rows, pgx.RowToStructByName[types.OrderRecord])
	if err != nil {
		return nil, fmt.Errorf("failed unpacking rows %w", err)
	}
	return orders, nil
}

func (d *Database) InsertWithdrawAndUpdateBalance(ctx context.Context, userID int, order string, sum float64) error {
	query := `
	    UPDATE balance
		SET current = current - $1,
		    withdrawn = withdrawn + $1
		WHERE user_id = $2 AND current >= $1
		RETURNING 1
	`
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, query, sum, userID)

	var success int
	if err := row.Scan(&success); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w", ErrNotEnoughBalance)
		}
		return fmt.Errorf("unexpected DB error %w", err)
	}
	query = `
		INSERT INTO withdrawal (user_id, order_name, sum)
		VALUES ($1, $2, $3)
	`
	_, err = tx.Exec(ctx, query, userID, order, sum)
	if err != nil {
		return fmt.Errorf("unexpected DB error %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func (d *Database) UpdateUnprocessedOrder(ctx context.Context, orderID int, newStatus types.Status, accrual int) error {
	query := `
		UPDATE user_order
		SET status = $1, accrual = $2
		WHERE id = $3
		AND status not in ('INVALID', 'PROCESSED')
		RETURNING user_id`

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, query, newStatus, accrual, orderID)

	var userID int
	if err := row.Scan(&userID); err != nil {
		return fmt.Errorf("%w", err)
	}

	query = `
		INSERT INTO balance (user_id, current, withdrawn)
		VALUES ($1, $2, 0)
		ON CONFLICT(user_id)
		DO UPDATE SET current = balance.current + $2
	`
	_, err = tx.Exec(ctx, query, userID, accrual)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func (d *Database) GetUserWithdrawals(ctx context.Context, userID int) ([]types.Withdrawal, error) {
	query := `
		SELECT sum, order_name, processed_at
		FROM withdrawal
		WHERE user_id = $1
		ORDER BY id
		LIMIT 1000
		`
	rows, err := d.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed collecting rows %w", err)
	}

	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[types.Withdrawal])
	if err != nil {
		return nil, fmt.Errorf("failed unpacking rows %w", err)
	}
	return results, nil
}

func (d *Database) GetUserOrders(ctx context.Context, userID int) ([]types.OrderInfo, error) {

	query := `
		SELECT order_number, status, accrual, uploaded_at
		FROM user_order
		WHERE user_id = $1
		ORDER BY id
		LIMIT 1000
	`
	rows, err := d.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed collecting rows %w", err)
	}

	orders, err := pgx.CollectRows(rows, pgx.RowToStructByName[types.OrderInfo])
	if err != nil {
		return nil, fmt.Errorf("failed unpacking rows %w", err)
	}
	return orders, nil
}

func (d *Database) GetUserBalance(ctx context.Context, userID int) (*types.Balance, error) {

	query := `
		SELECT current, withdrawn
		FROM balance
		WHERE user_id = $1
	`
	rows, err := d.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed collecting rows %w", err)
	}

	balance, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[types.Balance])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &types.Balance{}, nil
		}
		return nil, fmt.Errorf("failed unpacking rows %w", err)
	}
	return &balance, nil
}
