package main

import (
	"testing"
)

func twoSum(nums []int, target int) []int {
	m := make(map[int]int)
	for i, n := range nums {
		if j, ok := m[target-n]; ok {
			return []int{j, i}
		}
		m[n] = i
	}
	return nil
}

func TestTwoSum(t *testing.T) {
	tests := []struct {
		nums   []int
		target int
		want   []int
	}{
		{[]int{2, 7, 11, 15}, 9, []int{0, 1}},
		{[]int{3, 2, 4}, 6, []int{1, 2}},
		{[]int{3, 3}, 6, []int{0, 1}},
		{[]int{1, 5, 3, 7, 2}, 9, []int{3, 4}},
	}

	for _, tt := range tests {
		got := twoSum(tt.nums, tt.target)
		if len(got) != len(tt.want) || got[0] != tt.want[0] || got[1] != tt.want[1] {
			t.Errorf("twoSum(%v, %d) = %v, want %v", tt.nums, tt.target, got, tt.want)
		}
	}
}