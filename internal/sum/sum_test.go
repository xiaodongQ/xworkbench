package sum

import (
	"reflect"
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

func TestTwoSum(t *testing.T) {
	tests := []struct {
		name     string
		nums     []int
		target   int
		expected []int
	}{
		{"基础用例", []int{2, 7, 11, 15}, 9, []int{0, 1}},
		{"中间两数", []int{3, 2, 4}, 6, []int{1, 2}},
		{"重复元素", []int{3, 3}, 6, []int{0, 1}},
		{"含负数", []int{-3, 4, 3, 90}, 0, []int{0, 2}},
		{"仅两个元素", []int{1, 2}, 3, []int{0, 1}},
		{"首尾配对", []int{1, 2, 3, 4, 9}, 10, []int{0, 4}},
		{"首尾配对-全数组", []int{1, 2, 4, 7, 11}, 12, []int{0, 4}},
		{"含零", []int{0, 4, 3, 0}, 0, []int{0, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TwoSum(tt.nums, tt.target)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("TwoSum(%v, %d) = %v, want %v", tt.nums, tt.target, got, tt.expected)
			}
		})
	}
}
