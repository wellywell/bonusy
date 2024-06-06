package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	pool *pgxpool.Pool
}

func NewDatabase(connString string) (*Database, error) {

	err := Migrate(connString)

	if err != nil {
		return nil, fmt.Errorf("Failed to migrate %w", err)
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

func (d *Database) InsertUserOrder(ctx context.Context, order string, userID int, status string) error {

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
