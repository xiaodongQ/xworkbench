package executor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/xiaodongQ/xworkbench/internal/logger"
)

// aiProtectedPaths AI 任务禁止写入的源码路径前缀。post-write validation
// 发现这些路径下出现新文件/改动，会自动清理并 warn。
//
// 设计原则：只列源码树路径（会被 go build 编进二进制），不列 data/、
// .claude/ 等其他 gitignore 项（那些是合理 destination）。
var aiProtectedPaths = []string{
	"cmd/",
	"internal/",
	"openspec/",
}

// ValidateAIWrites 检查 cwd 下 working tree 是否有 AI 写出的文件落到
// 受保护路径（cmd/ internal/ openspec/），有则自动回滚（untracked rm /
// modified git checkout）并 warn。
//
// 这是 post-write 兜底：executor.RunInSandbox 把 CWD 强制到沙盒目录
// 防 AI 写源码树，但 AI 可能用绝对路径或别的 trick 绕开。RunInSandbox
// 跑完会自动调一次，捕获漏网之鱼。
//
// 行为：
//   - 不是 git repo / git 不可用：跳过（不阻断任务）
//   - 受保护路径有变更：rm 或 checkout + warn log
//   - 受保护路径无变更：no-op
func ValidateAIWrites() {
	// --untracked-files=all 让 git 把 untracked 目录展开成每个文件单独一行，
	// 否则 internal/ 下新文件 git 只报 ?? internal/ 一行，没法精准 rm
	out, err := exec.Command("git", "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		// 非 git 环境（开发/单文件工具模式），不做检查
		return
	}
	cwd, _ := os.Getwd()
	var unwanted []change
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if len(line) < 3 {
			continue
		}
		status := line[:2]
		path := strings.TrimSpace(line[2:])
		// 去掉 git 的引号包裹（路径含空格时）
		path = strings.Trim(path, "\"")
		if isAIProtectedPath(path) {
			unwanted = append(unwanted, change{status, path})
		}
	}
	if len(unwanted) == 0 {
		return
	}
	// 清理
	for _, c := range unwanted {
		abs := filepath.Join(cwd, c.path)
		if c.status[0] == '?' || c.status[0] == 'A' {
			// Untracked 或 staged-but-uncommitted-new: rm
			_ = os.Remove(abs)
		} else {
			// Modified / deleted: 恢复
			_ = exec.Command("git", "checkout", "--", c.path).Run()
		}
	}
	logger.Logger.Warnw("ai task wrote outside sandbox; reverted",
		"reverted_files", pathList(unwanted),
		"expected_dir", "data/ai-sandbox/",
		"hint", "AI 任务应写到 data/ai-sandbox/，写到 internal/ 等源码路径会被自动回滚")
}

func isAIProtectedPath(relPath string) bool {
	for _, prefix := range aiProtectedPaths {
		if strings.HasPrefix(relPath, prefix) {
			return true
		}
	}
	return false
}

func pathList(changes []change) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, c.path)
	}
	return out
}

// change 是 ValidateAIWrites 内部用的 git status 解析结果。
type change struct {
	status string // porcelain 2 字符
	path   string // 相对 cwd
}
