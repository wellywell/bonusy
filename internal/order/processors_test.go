package order

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wellywell/bonusy/internal/accrual"
	"github.com/wellywell/bonusy/internal/order/mocks"
)

/***func TestCheckAccrualOrders(t *testing.T) {

	c := mocks.NewAccrualClient(t)

	type args struct {
		ctx   context.Context
		tasks <-chan types.OrderRecord
	}
	tests := []struct {
		name string
		args args
		want chan OrderUpdate
	}{
		{}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			timeOutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)

			c.EXPECT().Get("some path").Return("result", nil)

			if got := CheckAccrualOrders(tt.args.ctx, tt.args.tasks, c); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CheckAccrualOrders() = %v, want %v", got, tt.want)
			}
		})
	}
}
***/

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
