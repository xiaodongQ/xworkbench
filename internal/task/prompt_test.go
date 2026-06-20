package task

import (
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// TestBuildTaskPrompt_NoExperience 验证 0 经验时不写经验库段落（向后兼容）。
func TestBuildTaskPrompt_NoExperience(t *testing.T) {
	task := &backend.Task{
		Title:       "纯任务",
		Description: "无经验关联",
	}
	got := BuildTaskPrompt(task)
	if strings.Contains(got, "相关经验库") {
		t.Errorf("should NOT contain 相关经验库 when no experience:\n%s", got)
	}
	if !strings.Contains(got, "纯任务") || !strings.Contains(got, "无经验关联") {
		t.Errorf("missing task title/description:\n%s", got)
	}
}

// TestBuildTaskPrompt_SingleExperience 验证 1 个经验时退化旧行为（无 index 后缀）。
// 目的：保证现有单经验任务的 prompt diff 最小（evaluator 不会因为 prompt 微小变化而误判）。
func TestBuildTaskPrompt_SingleExperience(t *testing.T) {
	task := &backend.Task{Title: "T", Description: "D"}
	exp := &backend.Experience{
		Module:   "redis",
		Scene:    "集群失联",
		Keywords: "MOVED,ASK",
	}
	got := BuildTaskPrompt(task, exp)
	if !strings.Contains(got, "## 分类\nredis\n") {
		t.Errorf("single exp should keep plain '## 分类' (no index suffix), got:\n%s", got)
	}
	if strings.Contains(got, "（1/1") {
		t.Errorf("single exp should NOT have index suffix, got:\n%s", got)
	}
}

// TestBuildTaskPrompt_MultiExperience 验证 2+ 个经验时全部注入且带 index。
func TestBuildTaskPrompt_MultiExperience(t *testing.T) {
	task := &backend.Task{Title: "复合任务", Description: "需要 redis + k8s 经验"}
	exp1 := &backend.Experience{
		Module:   "redis-cluster",
		Scene:    "集群节点失联",
		Keywords: "CLUSTERDOWN,MOVED",
	}
	exp2 := &backend.Experience{
		Module:   "k8s-oom",
		Scene:    "Pod OOM Killed 定位",
		Keywords: "OOMKilled,exit code 137",
	}
	exp3 := &backend.Experience{
		Module:   "mysql-deadlock",
		Scene:    "行锁等待与死锁分析",
		Keywords: "LOCK WAIT,deadlock found",
	}

	got := BuildTaskPrompt(task, exp1, exp2, exp3)

	// 必须包含每条经验的模块
	for _, mod := range []string{"redis-cluster", "k8s-oom", "mysql-deadlock"} {
		if !strings.Contains(got, mod) {
			t.Errorf("missing module %q in prompt:\n%s", mod, got)
		}
	}
	// 必须包含每条经验的 scene
	for _, scene := range []string{"集群节点失联", "Pod OOM Killed 定位", "行锁等待与死锁分析"} {
		if !strings.Contains(got, scene) {
			t.Errorf("missing scene %q in prompt", scene)
		}
	}
	// 多经验时必须有 index 后缀（1/3、2/3、3/3）
	for _, idx := range []string{"（1/3，模块: redis-cluster）", "（2/3，模块: k8s-oom）", "（3/3，模块: mysql-deadlock）"} {
		if !strings.Contains(got, idx) {
			t.Errorf("missing index suffix %q in prompt", idx)
		}
	}
}

// TestBuildTaskPrompt_MultiExperience_SkipsNil 验证 nil experience 元素被跳过。
func TestBuildTaskPrompt_MultiExperience_SkipsNil(t *testing.T) {
	task := &backend.Task{Title: "T", Description: "D"}
	exp1 := &backend.Experience{Module: "redis", Scene: "scene-A"}
	exp2 := &backend.Experience{Module: "k8s", Scene: "scene-B"}

	got := BuildTaskPrompt(task, exp1, nil, exp2)
	if !strings.Contains(got, "scene-A") || !strings.Contains(got, "scene-B") {
		t.Errorf("valid exps should be in prompt:\n%s", got)
	}
	// 不应因为 nil 多生成一个 index
	if strings.Contains(got, "（2/4") {
		t.Errorf("nil exp should be skipped (not count toward index):\n%s", got)
	}
	// 索引应是 1/2、2/2
	if !strings.Contains(got, "（1/2") || !strings.Contains(got, "（2/2") {
		t.Errorf("index should be 1/2 and 2/2 (nil skipped):\n%s", got)
	}
}