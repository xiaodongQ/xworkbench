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

	skillDir := skill.Dir
	scriptPath := filepath.Join(skillDir, skill.XWCommand)
	if strings.HasPrefix(skill.XWCommand, "/") {
		// 绝对路径，直接用
		scriptPath = skill.XWCommand
	}

	// 检查脚本是否存在
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		// 尝试在 skillDir 下找（如果 xw_command 是相对路径）
		if !strings.Contains(skill.XWCommand, "/") {
			scriptPath = filepath.Join(skillDir, skill.XWCommand)
		}
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("script not found: %s", skill.XWCommand)
		}
	}

	// 序列化输入为 JSON
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal input: %w", err)
	}

	// 执行命令（shell 解析 command）
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", skill.XWCommand)
	cmd.Dir = skillDir
	cmd.Stdin = bytes.NewReader(inputBytes)
	cmd.Env = append(os.Environ(), "SKILL_INPUT_STDIN=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	rawOut := stdout.String()
	rawErr := stderr.String()

	result := &ExecuteSkillResult{
		RawOut: rawOut,
		RawErr: rawErr,
	}

	if err != nil {
		result.Status = "error"
		// 尝试从 stderr/stdout 解析 JSON 格式错误信息
		var errOut map[string]any
		if json.Unmarshal([]byte(rawOut), &errOut) == nil {
			result.Output = errOut
		} else {
			result.Output = map[string]any{"error": rawErr}
		}
		return result, fmt.Errorf("skill %s failed: %w", skill.Name, err)
	}

	// 解析 stdout JSON
	var output map[string]any
	if err := json.Unmarshal([]byte(rawOut), &output); err != nil {
		// 输出不是 JSON，记录 raw 并标记
		result.Output = map[string]any{"raw": rawOut}
	}

	result.Status = "ok"
	if result.Output == nil {
		result.Output = output
	}

	return result, nil
}
