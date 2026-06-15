package sum

import (
	"testing"
)

func TestSum(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{"正数相加", 1, 2, 3},
		{"负数相加", -1, -2, -3},
		{"正负相加", -5, 3, -2},
		{"零相加", 0, 5, 5},
		{"大数相加", 1000000, 2000000, 3000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sum(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("Sum(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}