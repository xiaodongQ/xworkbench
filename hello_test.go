package main

import (
	"reflect"
	"testing"
)

func TestAddInt(t *testing.T) {
	cases := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"正数相加", 2, 3, 5},
		{"负数加正数", -1, 1, 0},
		{"零相加", 0, 5, 5},
		{"两个零", 0, 0, 0},
		{"负数相加", -3, -4, -7},
		{"大数相加", 1000000, 2000000, 3000000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := addInt(c.a, c.b)
			if got != c.want {
				t.Errorf("addInt(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

// 两数之和（LeetCode 1）：在 nums 中找出和为 target 的两个不同下标。
func TestTwoSum(t *testing.T) {
	cases := []struct {
		name   string
		nums   []int
		target int
		want   []int
	}{
		{"基本用例", []int{2, 7, 11, 15}, 9, []int{0, 1}},
		{"中段命中", []int{3, 2, 4}, 6, []int{1, 2}},
		{"重复元素", []int{3, 3}, 6, []int{0, 1}},
		{"含负数", []int{-1, -2, -3, -4, -5}, -8, []int{2, 4}},
		{"零参与", []int{0, 4, 3, 0}, 0, []int{0, 3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := twoSum(c.nums, c.target)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("twoSum(%v, %d) = %v, want %v", c.nums, c.target, got, c.want)
			}
		})
	}
}