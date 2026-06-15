package sum

func Sum(a, b int) int {
	return a + b
}

// TwoSum 返回 nums 中和为 target 的两个元素的下标(升序)。
// 使用哈希表一次遍历,时间 O(n),空间 O(n)。
func TwoSum(nums []int, target int) []int {
	seen := make(map[int]int, len(nums))
	for i, n := range nums {
		if j, ok := seen[target-n]; ok {
			return []int{j, i}
		}
		seen[n] = i
	}
	return nil
}
