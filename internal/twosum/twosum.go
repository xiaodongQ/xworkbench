// Package twosum 求解"两数之和":在数组中找出和为目标值的两个下标。
package twosum

// TwoSum 返回 nums 中两个元素的下标,使其相加等于 target。
// 同一元素不能重复使用。保证仅有一组解。
//
// 算法:一次哈希表。遍历 nums,对每个 n,先查 target-n 是否已在 seen 中;
// 若在,直接返回 [seen[target-n], 当前下标];否则把 n 记入 seen。
// 时间 O(n),空间 O(n)。
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
