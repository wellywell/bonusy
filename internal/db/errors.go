package db

import (
	"errors"
	"fmt"
)

var ErrNotEnoughBalance = errors.New("not enough balance")

type UserExistsError struct {
	Username string
}

func (e *UserExistsError) Error() string {
	return fmt.Sprintf("User %s exists", e.Username)
}

type UserNotFoundError struct {
	Username string
}

func (e *UserNotFoundError) Error() string {
	return fmt.Sprintf("User %s not found", e.Username)
}

type UserAlreadyUploadedOrder struct {
	UserID int
	Order  string
}

func (e *UserAlreadyUploadedOrder) Error() string {
	return fmt.Sprintf("User %d already uploaded order %s", e.UserID, e.Order)
}

type OrderUploadedByWrongUser struct {
	Order string
}

func (e *OrderUploadedByWrongUser) Error() string {
	return fmt.Sprintf("Other user already uploaded order %s", e.Order)
}
