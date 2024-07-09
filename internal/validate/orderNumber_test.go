package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVaildateOrderNumber(t *testing.T) {

	testCases := []struct {
		number string
		result bool
	}{
		{"1111", false},
		{"1234567812345670", true},
		{"1111222233334444", true},
		{"1111222233334441", false},
		{"49927398716", true},
		{"49927398717", false},
		{"1234567812345678", false},
		{"79927398710", false},
		{"79927398711", false},
		{"79927398712", false},
		{"79927398713", true},
		{"79927398714", false},
		{"79927398715", false},
		{"79927398716", false},
		{"79927398717", false},
		{"79927398718", false},
		{"79927398719", false},
		{"8", false},
		{"0", true},
		{"", false},
		{"letter", false},
	}

	for _, tc := range testCases {
		t.Run(tc.number, func(t *testing.T) {
			result := ValidateOrderNumber(tc.number)

			assert.Equal(t, tc.result, result)
		})
	}
}
