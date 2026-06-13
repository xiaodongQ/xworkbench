package main

import "fmt"

// addInt 求两个整数之和。
func addInt(a, b int) int {
	return a + b
}

// twoSum 在 nums 中找出和为 target 的两个不同下标（哈希表 O(n)）。
func twoSum(nums []int, target int) []int {
	seen := make(map[int]int, len(nums))
	for i, n := range nums {
		if j, ok := seen[target-n]; ok {
			return []int{j, i}
		}
		seen[n] = i
	}
	return nil
}

func main() {
	fmt.Println("Hello, World!")
	fmt.Printf("addInt(2, 3) = %d\n", addInt(2, 3))
	fmt.Printf("twoSum([2,7,11,15], 9) = %v\n", twoSum([]int{2, 7, 11, 15}, 9))
}
