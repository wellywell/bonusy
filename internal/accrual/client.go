package accrual

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/wellywell/bonusy/internal/types"
)

type AccrualClient struct {
	address string
}

type OrderStatus struct {
	Order   string       `json:"order"`
	Status  types.Status `json:"status"`
	Accrual int          `json:"accrual"`
}

type ErrThrottle struct {
	RetryAfter int
}

func (e *ErrThrottle) Error() string {
	return "Too many requests"
}

var (
	ErrUnknown        = errors.New("unknown server error")
	ErrOrderNotExists = errors.New("order not exists")
)

func NewAccrualClient(address string) *AccrualClient {
	return &AccrualClient{address: address}
}

func (c *AccrualClient) GetOrderStatus(orderNum string) (*OrderStatus, error) {

	url := fmt.Sprintf("%s/api/orders/%s", c.address, orderNum)

	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	switch response.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading body error %w", err)
		}
		var status OrderStatus
		err = json.Unmarshal(body, &status)
		if err != nil {
			return nil, fmt.Errorf("json parsing error %w", err)
		}
		return &status, nil

	case http.StatusNoContent:
		return nil, fmt.Errorf("%w", ErrOrderNotExists)
	case http.StatusTooManyRequests:
		sleepStr := response.Header.Get("Retry-After")
		sleep, err := strconv.Atoi(sleepStr)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w", &ErrThrottle{RetryAfter: sleep})
	case http.StatusInternalServerError:
		return nil, fmt.Errorf("%w", ErrUnknown)
	default:
		return nil, fmt.Errorf("unexpected status %d", response.StatusCode)
	}

}
