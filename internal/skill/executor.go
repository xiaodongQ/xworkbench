package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ExecuteSkill 执行 skill 脚本，输入参数通过 stdin JSON 传入。
// 工作目录为 skill 目录，输出为 stdout 的 JSON。
func ExecuteSkill(skill *Skill, input map[string]any) (*ExecuteSkillResult, error) {
	if skill.XWCommand == "" {
		return nil, fmt.Errorf("xw_command not set")
	}

	// 序列化输入为 JSON
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal input: %w", err)
	}

	// 执行命令（通过 shell，允许相对路径）
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", skill.XWCommand)
	cmd.Dir = skill.Dir

	// 注入 PYTHONPATH：让 skill 脚本可以 import 同级目录的共享模块
	// 例如 tools/http_util/scripts/http_util.py 可以被 tools/weather/scripts/check.py
	// 通过 "from http_util.scripts.http_util import ..." 引用
	toolsRoot := filepath.Dir(skill.Dir) // .../tools
	pythonPath := os.Getenv("PYTHONPATH")
	if pythonPath != "" {
		pythonPath = toolsRoot + string(os.PathListSeparator) + pythonPath
	} else {
		pythonPath = toolsRoot
	}
	env := append(os.Environ(), "SKILL_INPUT_STDIN=1", "PYTHONPATH="+pythonPath)
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdin = bytes.NewReader(inputBytes)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	rawOut := strings.TrimSpace(stdout.String())
	rawErr := strings.TrimSpace(stderr.String())

	result := &ExecuteSkillResult{
		RawOut: rawOut,
		RawErr: rawErr,
	}

	if err != nil {
		result.Status = "error"
		// 尝试从 stdout 解析 JSON
		var outMap map[string]any
		if rawOut != "" && json.Unmarshal([]byte(rawOut), &outMap) == nil {
			result.Output = outMap
		} else {
			result.Output = map[string]any{"error": rawErr}
		}
		return result, fmt.Errorf("skill %s failed: %w", skill.Name, err)
	}

	// 解析 stdout JSON
	var output map[string]any
	if rawOut != "" && json.Unmarshal([]byte(rawOut), &output) == nil {
		result.Output = output
	}

	result.Status = "ok"
	return result, nil
}

// Execute 根据 skill 名字和输入参数执行技能。
// 内部自动从 registry 查找 skill，找不到返回 error。
func Execute(name string, input map[string]any) (*ExecuteSkillResult, error) {
	s := GetByName(name)
	if s == nil {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	return ExecuteSkill(s, input)
}
