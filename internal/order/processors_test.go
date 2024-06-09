package order

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/wellywell/bonusy/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/wellywell/bonusy/internal/accrual"
	"github.com/wellywell/bonusy/internal/order/mocks"
)

func TestCheckAccrualOrders(t *testing.T) {

	c := mocks.NewAccrualClient(t)

	tests := []struct {
		name           string
		result         *accrual.OrderStatus
		wantError      error
		expectedResult *OrderUpdate
	}{
		{"no change", &accrual.OrderStatus{Order: "123", Status: "NEW"}, nil, nil},
		{"changed", &accrual.OrderStatus{Order: "123", Status: "INVALID"},
			nil, &OrderUpdate{
				order:  types.OrderRecord{OrderNum: "123", Status: "NEW", OrderID: 1},
				status: accrual.OrderStatus{Order: "123", Status: "INVALID", Accrual: 0}},
		},
		{"processed", &accrual.OrderStatus{Order: "123", Status: "PROCESSED", Accrual: 500},
			nil, &OrderUpdate{
				order:  types.OrderRecord{OrderNum: "123", Status: "NEW", OrderID: 1},
				status: accrual.OrderStatus{Order: "123", Status: "PROCESSED", Accrual: 500}},
		},
		{"error", nil, fmt.Errorf("Some error"), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			timeOutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
			defer cancel()

			c.EXPECT().GetOrderStatus("123").Return(tt.result, tt.wantError).Once()

			inp := make(chan types.OrderRecord)
			out := CheckAccrualOrders(timeOutCtx, inp, c)

			inp <- types.OrderRecord{OrderNum: "123", Status: "NEW", OrderID: 1}

			val := <-out
			if tt.expectedResult != nil {
				assert.Equal(t, *tt.expectedResult, val)
			}

		})
	}
}

func Test_retryThrottle(t *testing.T) {

	c := mocks.NewAccrualClient(t)

	tests := []struct {
		name          string
		result        *accrual.OrderStatus
		wantError     error
		throttleTimes int
	}{
		{"no throttle", &accrual.OrderStatus{Order: "1", Status: "1", Accrual: 1}, nil, 0},
		{"throttle once", &accrual.OrderStatus{}, &accrual.ErrThrottle{RetryAfter: 1}, 1},
		{"throttle twice", &accrual.OrderStatus{}, &accrual.ErrThrottle{RetryAfter: 1}, 2},
		{"other error", nil, fmt.Errorf("Some error"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.wantError != nil && tt.throttleTimes > 0 {
				for _ = range tt.throttleTimes {
					c.EXPECT().GetOrderStatus("123").Return(nil, tt.wantError).Once()
				}
				c.EXPECT().GetOrderStatus("123").Return(tt.result, nil).Once()
			} else {
				c.EXPECT().GetOrderStatus("123").Return(tt.result, tt.wantError).Once()
			}

			got, err := retryThrottle("123", c)
			if tt.wantError == nil {
				assert.NoError(t, err)
			} else {
				var errThrottle *accrual.ErrThrottle
				if errors.As(tt.wantError, &errThrottle) {
					assert.NoError(t, err)
				} else {
					assert.EqualError(t, err, tt.wantError.Error())
				}
			}
			assert.Equal(t, got, tt.result)

		})
	}
}
