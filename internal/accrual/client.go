package accrual

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wellywell/bonusy/internal/types"
	"io"
	"net/http"
)

type AccrualClient struct {
	address string
}

type OrderStatus struct {
	Order   string       `json:"order"`
	Status  types.Status `json:"status"`
	Accrual int          `json:"accrual"`
}

var (
	ThrottleError  = errors.New("Too many requests")
	UnknownError   = errors.New("Unknown server error")
	OrderNotExists = errors.New("Order not exists")
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
		return nil, fmt.Errorf("%w", OrderNotExists)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("%w", ThrottleError)
	case http.StatusInternalServerError:
		return nil, fmt.Errorf("%w", UnknownError)
	default:
		return nil, fmt.Errorf("Unexpected status %d", response.StatusCode)
	}

}
