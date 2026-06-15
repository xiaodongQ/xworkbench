package twosum

import (
	"reflect"
	"testing"
)

func TestTwoSum(t *testing.T) {
	tests := []struct {
		name   string
		nums   []int
		target int
		want   []int
	}{
		{"官方示例", []int{2, 7, 11, 15}, 9, []int{0, 1}},
		{"中间配对", []int{3, 2, 4}, 6, []int{1, 2}},
		{"重复元素", []int{3, 3}, 6, []int{0, 1}},
		{"负数", []int{-1, -2, -3, -4, -5}, -8, []int{2, 4}},
		{"首尾配对", []int{1, 5, 3, 7, 9}, 10, []int{2, 3}},
		{"含零", []int{0, 4, 3, 0}, 0, []int{0, 3}},
		{"大数", []int{1000000, 2000000, 3000000}, 3000000, []int{0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TwoSum(tt.nums, tt.target)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TwoSum(%v, %d) = %v, want %v", tt.nums, tt.target, got, tt.want)
			}
		})
	}
}

func TestTwoSumNoSolution(t *testing.T) {
	if got := TwoSum([]int{1, 2, 3}, 100); got != nil {
		t.Errorf("无解时应返回 nil, 得到 %v", got)
	}
}

func TestTwoSumSingleElement(t *testing.T) {
	if got := TwoSum([]int{5}, 5); got != nil {
		t.Errorf("单元素无解时应返回 nil, 得到 %v", got)
	}
}

func BenchmarkTwoSum(b *testing.B) {
	nums := make([]int, 1000)
	for i := range nums {
		nums[i] = i
	}
	target := nums[500] + nums[999]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TwoSum(nums, target)
	}
}
