package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ─── SKILL.md 解析测试 ────────────────────────────────────────────────────────

func TestParseSkillMeta(t *testing.T) {
	// 创建临时 skill 目录
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "SKILL.md")
	err := os.WriteFile(skillFile, []byte(skillMDExample), 0644)
	if err != nil {
		t.Fatal(err)
	}

	meta, err := ParseSkillMeta(skillFile)
	if err != nil {
		t.Fatalf("ParseSkillMeta failed: %v", err)
	}

	if meta.Name != "nginx-check" {
		t.Errorf("Name: want nginx-check, got %s", meta.Name)
	}
	if meta.Description == "" {
		t.Error("Description should not be empty")
	}
	if meta.XWCommand == "" {
		t.Error("XWCommand should not be empty")
	}
	if meta.XWCommand != "python3 scripts/check.py" {
		t.Errorf("XWCommand: want 'python3 scripts/check.py', got %s", meta.XWCommand)
	}
	if meta.XWParams == nil {
		t.Fatal("XWParams should not be nil")
	}
	if meta.XWParams["host"] != "Nginx 主机地址（必填）" {
		t.Errorf("XWParams[host]: got %s", meta.XWParams["host"])
	}
	if len(meta.XWExamples) != 2 {
		t.Errorf("XWExamples count: want 2, got %d", len(meta.XWExamples))
	}
}

func TestParseSkillMetaMissingFile(t *testing.T) {
	_, err := ParseSkillMeta("/nonexistent/SKILL.md")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseSkillMetaInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "SKILL.md")
	// 故意写一个 YAML 解析会失败的 frontmatter
	os.WriteFile(skillFile, []byte("---naked yaml without dashes---\n\n# Body"), 0644)

	_, err := ParseSkillMeta(skillFile)
	if err == nil {
		t.Error("expected error for invalid YAML frontmatter")
	}
}

func TestParseSkillMetaMissingName(t *testing.T) {
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "SKILL.md")
	os.WriteFile(skillFile, []byte(`---
description: test only
xw_command: python3 run.py
---
`), 0644)

	_, err := ParseSkillMeta(skillFile)
	if err == nil {
		t.Error("expected error for missing name field")
	}
}

// ─── 扫描测试 ─────────────────────────────────────────────────────────────────

func TestScanToolsDir(t *testing.T) {
	// 创建模拟 tools/ 目录结构
	toolsDir := t.TempDir()

	// skill 1: 完整
	skill1 := filepath.Join(toolsDir, "skill-one")
	os.Mkdir(skill1, 0755)
	os.Mkdir(filepath.Join(skill1, "scripts"), 0755)
	os.WriteFile(filepath.Join(skill1, "SKILL.md"), []byte(`---
name: skill-one
description: Test skill one
xw_command: python3 scripts/check.py
xw_params:
  host: 主机地址
xw_output:
  status: ok | error
xw_examples: []
---
`), 0644)
	os.WriteFile(filepath.Join(skill1, "scripts", "check.py"), []byte("# dummy"), 0644)

	// skill 2: 缺少 SKILL.md
	skill2 := filepath.Join(toolsDir, "skill-two")
	os.Mkdir(skill2, 0755)
	os.WriteFile(filepath.Join(skill2, "README.md"), []byte("no skill"), 0644)

	// skill 3: 目录里只有隐藏文件
	skill3 := filepath.Join(toolsDir, "skill-three")
	os.Mkdir(skill3, 0755)
	os.WriteFile(filepath.Join(skill3, ".gitkeep"), []byte(""), 0644)

	skills, err := ScanToolsDir(toolsDir)
	if err != nil {
		t.Fatalf("ScanToolsDir failed: %v", err)
	}

	// 只应该发现 skill-one
	if len(skills) != 1 {
		t.Errorf("want 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "skill-one" {
		t.Errorf("want skill-one, got %s", skills[0].Name)
	}
}

func TestScanToolsDirEmpty(t *testing.T) {
	emptyDir := t.TempDir()
	skills, err := ScanToolsDir(emptyDir)
	if err != nil {
		t.Fatalf("ScanToolsDir on empty dir failed: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("want 0 skills, got %d", len(skills))
	}
}

// ─── 执行测试 ─────────────────────────────────────────────────────────────────

func TestExecuteSkill(t *testing.T) {
	if os.Getenv("TEST_EXEC") == "" {
		t.Skip("skipping execution test (set TEST_EXEC=1 to run)")
	}

	// 创建模拟工具脚本
	toolsDir := t.TempDir()
	toolDir := filepath.Join(toolsDir, "test-tool")
	os.Mkdir(toolDir, 0755)
	os.Mkdir(filepath.Join(toolDir, "scripts"), 0755)

	// 写一个输出 JSON 的脚本（兼容 bash + python）
	script := filepath.Join(toolDir, "scripts", "check.py")
	os.WriteFile(script, []byte(`#!/usr/bin/env python3
import sys, json
params = json.load(sys.stdin)
print(json.dumps({"status": "ok", "received_host": params.get("host", ""), "code": 200}))
`), 0644)

	skill := &Skill{
		Name:        "test-tool",
		Dir:         toolDir,
		XWCommand:   "python3 scripts/check.py",
		XWParams:    map[string]string{"host": "主机地址"},
		XWOutput:    map[string]string{"status": "ok | error"},
		XWExamples:  nil,
	}

	input := map[string]any{"host": "example.com"}
	result, err := ExecuteSkill(skill, input)
	if err != nil {
		t.Fatalf("ExecuteSkill failed: %v", err)
	}

	if result.Status != "ok" {
		t.Errorf("Status: want ok, got %s", result.Status)
	}
	if result.Output["received_host"] != "example.com" {
		t.Errorf("received_host: want example.com, got %s", result.Output["received_host"])
	}
}

func TestExecuteSkillScriptNotFound(t *testing.T) {
	if os.Getenv("TEST_EXEC") == "" {
		t.Skip("skipping execution test (set TEST_EXEC=1 to run)")
	}

	toolDir := t.TempDir()
	skill := &Skill{
		Name:      "missing-tool",
		Dir:       toolDir,
		XWCommand: "python3 nonexistent.py",
	}

	_, err := ExecuteSkill(skill, nil)
	if err == nil {
		t.Error("expected error for nonexistent script")
	}
}

// ─── AI Tools 导出测试 ────────────────────────────────────────────────────────

func TestSkillToTool(t *testing.T) {
	skill := &Skill{
		Name:            "nginx-check",
		Description:     "检查 Nginx 状态",
		XWParams:        map[string]string{"host": "主机", "port": "端口"},
		XWOutput:        map[string]string{"status": "ok | error"},
		XWExamples:      []Example{{Description: "check localhost", Params: map[string]any{"host": "localhost"}}},
	}

	tool := skill.ToTool()
	if tool.Name != "nginx-check" {
		t.Errorf("tool.Name: got %s", tool.Name)
	}
	if tool.Description != "检查 Nginx 状态" {
		t.Errorf("tool.Description: got %s", tool.Description)
	}
	// params 应该被转成 JSON Schema 格式
	paramsBytes, _ := json.Marshal(tool.InputSchema)
	var schema map[string]any
	json.Unmarshal(paramsBytes, &schema)
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}
	if _, ok := props["host"]; !ok {
		t.Error("properties should have 'host' field")
	}
}

// ─── Helper ───────────────────────────────────────────────────────────────────

const skillMDExample = `---
name: nginx-check
description: 检查远程 Nginx 服务状态。当用户说"检查nginx"时触发。
version: 1.0.0
xw_command: python3 scripts/check.py
xw_params:
  host: Nginx 主机地址（必填）
  port: 端口，默认 80
  path: 检测路径，默认 /
xw_output:
  status: ok | error
  code: HTTP 状态码
  time_ms: 响应时间（毫秒）
xw_examples:
  - description: 检查本机 Nginx
    params: { host: localhost }
  - description: 检查远程 HTTPS
    params: { host: example.com, port: 443, path: /health }
---

# Nginx Check Skill

## 功能
检测 Nginx 服务是否正常运行。
`
