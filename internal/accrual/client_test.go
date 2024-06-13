package accrual

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClient(t *testing.T) {

	testCases := []struct {
		body            string
		code            int
		headers         map[string]string
		expectedErrorIs error
		expectedErrorAs error
		expectedResult  *OrderStatus
	}{
		{body: `{"order": "123", "status": "PROCESSED", "accrual": 500}`, code: http.StatusOK, headers: map[string]string{"Content-Type": "application/json"}, expectedResult: &OrderStatus{Order: "123", Status: "PROCESSED", Accrual: 500}},
		{body: `{"order": "123", "status": "REGISTERED"}`, code: http.StatusOK, headers: map[string]string{"Content-Type": "application/json"}, expectedResult: &OrderStatus{Order: "123", Status: "REGISTERED", Accrual: 0}},
		{body: `{"order": "123", "status": "PROCESSED"}`, code: http.StatusOK, headers: map[string]string{"Content-Type": "application/json"}, expectedResult: &OrderStatus{Order: "123", Status: "PROCESSED", Accrual: 0}},
		{body: `{"order": "123", "status": "INVALID"}`, code: http.StatusOK, headers: map[string]string{"Content-Type": "application/json"}, expectedResult: &OrderStatus{Order: "123", Status: "INVALID", Accrual: 0}},
		{body: `{"order": "123", "status": "PROCESSING"}`, code: http.StatusOK, headers: map[string]string{"Content-Type": "application/json"}, expectedResult: &OrderStatus{Order: "123", Status: "PROCESSING", Accrual: 0}},
		{body: "smth", code: http.StatusInternalServerError, expectedErrorIs: ErrUnknown},
		{body: "", code: http.StatusNoContent, expectedErrorIs: ErrOrderNotExists},
		{body: "No more than 0 requests per minute allowed", code: http.StatusTooManyRequests, headers: map[string]string{"Content-Type": "text/plain", "Retry-After": "1"}, expectedErrorAs: &ErrThrottle{}},
	}

	for _, tc := range testCases {
		t.Run(tc.body, func(t *testing.T) {
			svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, val := range tc.headers {
					w.Header().Set(key, val)
				}
				w.WriteHeader(tc.code)
				fmt.Fprintf(w, tc.body)
			}))
			defer svr.Close()
			c := NewAccrualClient(svr.URL)
			res, err := c.GetOrderStatus("123")
			if tc.expectedErrorIs != nil {
				assert.ErrorIs(t, err, tc.expectedErrorIs)
			} else if tc.expectedErrorAs != nil {
				assert.ErrorAs(t, err, &tc.expectedErrorAs)

				var errThrottle *ErrThrottle
				if errors.As(err, &errThrottle) {
					assert.NotEmpty(t, errThrottle.RetryAfter)
					retry, _ := strconv.Atoi(tc.headers["Retry-After"])
					assert.Equal(t, errThrottle.RetryAfter, retry)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, res)

		})
	}
}
