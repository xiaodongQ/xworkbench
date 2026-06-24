package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultConfig_EvalDefault 验证默认 config 含 claude/cbc 的 eval_default（评估默认模型）。
func TestDefaultConfig_EvalDefault(t *testing.T) {
	cfg := DefaultConfig()
	if got := cfg.Models["claude"].EvalDefault; got == "" {
		t.Errorf("claude.EvalDefault should be set in default config (got empty)")
	}
	if got := cfg.Models["cbc"].EvalDefault; got == "" {
		t.Errorf("cbc.EvalDefault should be set in default config (got empty)")
	}
	// eval_default 应从该 cli 的 options 列表里选（避免指向不存在的 model）
	for cli, group := range cfg.Models {
		found := false
		for _, opt := range group.Options {
			if opt.Value == group.EvalDefault {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s.EvalDefault=%q not in options list", cli, group.EvalDefault)
		}
	}
}

// TestMergeConfig_EvalDefault 验证 merge 行为：
// 1. src 有 eval_default → 覆盖 dst
// 2. src 无 eval_default → 保留 dst（向后兼容，老 config.json 不传 eval_default 不清空）
func TestMergeConfig_EvalDefault(t *testing.T) {
	t.Run("src_with_eval_default_overrides", func(t *testing.T) {
		dst := DefaultConfig()
		// 把 dst 的 claude.eval_default 改成 haiku（map 值需先取出再写回）
		dstCl := dst.Models["claude"]
		dstCl.EvalDefault = "haiku"
		dst.Models["claude"] = dstCl

		src := &Config{Models: ModelsConfig{
			"claude": {Default: "sonnet", EvalDefault: "opus"},
		}}
		mergeConfig(dst, src)
		if got := dst.Models["claude"].EvalDefault; got != "opus" {
			t.Errorf("claude.EvalDefault = %q, want %q (src should override)", got, "opus")
		}
	})

	t.Run("src_without_eval_default_preserves_dst", func(t *testing.T) {
		dst := DefaultConfig()
		dstCl := dst.Models["claude"]
		dstCl.EvalDefault = "haiku"
		dst.Models["claude"] = dstCl

		// 老 config.json 不含 eval_default 字段
		src := &Config{Models: ModelsConfig{
			"claude": {Default: "sonnet"},
		}}
		mergeConfig(dst, src)
		if got := dst.Models["claude"].EvalDefault; got != "haiku" {
			t.Errorf("claude.EvalDefault = %q, want %q (dst should be preserved)", got, "haiku")
		}
	})

	t.Run("roundtrip_json", func(t *testing.T) {
		// 模拟 Save→Load：写盘后读回，eval_default 字段不能丢
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		cfg := DefaultConfig()
		cfgCl := cfg.Models["claude"]
		cfgCl.EvalDefault = "opus"
		cfg.Models["claude"] = cfgCl
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := LoadFromPath(path); err != nil {
			t.Fatalf("LoadFromPath: %v", err)
		}
		if AppConfig == nil {
			t.Fatal("AppConfig should be set after LoadFromPath")
		}
		if got := AppConfig.Models["claude"].EvalDefault; got != "opus" {
			t.Errorf("after Save/Load, claude.EvalDefault = %q, want %q", got, "opus")
		}
	})
}

// TestMergeConfig_OmittedEvalDefaultKey 显式省略 eval_default JSON key（不是空字符串）时也应保留 dst。
// 这是向后兼容的关键：老 config.json 完全没这字段。
func TestMergeConfig_OmittedEvalDefaultKey(t *testing.T) {
	dst := DefaultConfig()
	dstCl := dst.Models["claude"]
	dstCl.EvalDefault = "haiku"
	dst.Models["claude"] = dstCl

	// 手写 JSON，eval_default 字段直接不写
	srcJSON := `{"models": {"claude": {"default": "sonnet", "options": [{"value":"haiku","label":"haiku"}]}}}`
	var src Config
	if err := json.Unmarshal([]byte(srcJSON), &src); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mergeConfig(dst, &src)
	if got := dst.Models["claude"].EvalDefault; got != "haiku" {
		t.Errorf("claude.EvalDefault = %q, want %q (空字符串 = 未设，保留 dst)", got, "haiku")
	}
}
