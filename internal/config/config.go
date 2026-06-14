package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// AppConfig 全局应用配置
var AppConfig *Config

// configFilePath 记录最近一次加载的配置文件路径，供 Save() 使用
var configFilePath string

type Config struct {
	Terminal TerminalConfig `json:"terminal"`
	Models   ModelsConfig   `json:"models"`
}

type TerminalConfig struct {
	DefaultType  string                      `json:"default_type"`  // 默认终端类型 key
	DetectPaths map[string][]string         `json:"detect_paths"` // 检测路径列表
	Types       map[string]TerminalTypeDef  `json:"types"`        // 终端类型定义
}

// TerminalTypeDef 终端类型定义
type TerminalTypeDef struct {
	Bin   string   `json:"bin"`   // 可执行文件名或路径
	Args  []string `json:"args"`  // 启动参数，{dir} 会被替换为目录路径
	Name  string   `json:"name"`  // 显示名称
	Plate string   `json:"plate"` // 适用平台：all / macOS / Windows / Linux
	Path  string   `json:"path"`  // 检测到的自定义路径
}

// ModelsConfig key=cli_type, value=模型配置
type ModelsConfig map[string]ModelGroup

type ModelGroup struct {
	Default string        `json:"default"`
	Options []ModelOption `json:"options"`
}

type ModelOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Load 加载配置（自动找路径或用默认）
// Save 将当前 AppConfig 写回 config.json
func Save() error {
	if AppConfig == nil {
		return nil
	}
	path := configFilePath
	if path == "" {
		path = configPath()
	}
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(AppConfig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func Load() (*Config, error) {
	cfg := DefaultConfig()
	path := configPath()
	if path == "" {
		return cfg, nil
	}
	return loadFromFile(path, cfg)
}

// LoadFromPath 从指定路径加载配置，覆盖全局 AppConfig
func LoadFromPath(path string) error {
	if path == "" {
		return nil
	}
	cfg := DefaultConfig()
	loaded, err := loadFromFile(path, cfg)
	if err != nil {
		return err
	}
	AppConfig = loaded
	configFilePath = path
	return nil
}

func loadFromFile(path string, cfg *Config) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		return cfg, err
	}
	mergeConfig(cfg, &loaded)
	return cfg, nil
}

func mergeConfig(dst, src *Config) {
	// terminal
	if src.Terminal.DefaultType != "" {
		dst.Terminal.DefaultType = src.Terminal.DefaultType
	}
	for k, v := range src.Terminal.DetectPaths {
		if len(v) > 0 {
			dst.Terminal.DetectPaths[k] = v
		}
	}
	for k, v := range src.Terminal.Types {
		// 只要有任何有效字段就合并
		if v.Bin != "" || len(v.Args) > 0 || v.Path != "" {
			dst.Terminal.Types[k] = v
		}
	}
	// models
	for cliType, srcGroup := range src.Models {
		dstGroup := dst.Models[cliType]
		if srcGroup.Default != "" {
			dstGroup.Default = srcGroup.Default
		}
		if len(srcGroup.Options) > 0 {
			dstGroup.Options = srcGroup.Options
		}
		dst.Models[cliType] = dstGroup
	}
}

func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	if dir == "/" || dir == "" {
		dir, _ = os.Getwd()
	}
	path := filepath.Join(dir, "config.json")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if runtime.GOOS == "darwin" {
		res := filepath.Join(dir, "..", "Resources", "config.json")
		if _, err := os.Stat(res); err == nil {
			return res
		}
	}
	return ""
}

// DefaultConfig 返回默认配置（含完整终端类型定义和模型列表）
func DefaultConfig() *Config {
	return &Config{
		Terminal: TerminalConfig{
			DefaultType:  "wezterm",
			DetectPaths:  map[string][]string{"wezterm": {"/Applications/WezTerm.app/Contents/MacOS/WezTerm"}},
			Types:        map[string]TerminalTypeDef{
				"wezterm": {Bin: "wezterm", Args: []string{"start", "--cwd", "{dir}", "--always-new-process"}, Name: "WezTerm", Plate: "all", Path: ""},
			},
		},
		Models: ModelsConfig{
			"claude": {Default: "sonnet", Options: []ModelOption{
				{Value: "haiku", Label: "haiku（快+便宜）"},
				{Value: "sonnet", Label: "sonnet（推荐 · 准确）"},
				{Value: "opus", Label: "opus（最强 · 贵）"},
			}},
			"cbc": {Default: "glm-5.1", Options: []ModelOption{
				{Value: "glm-5.1", Label: "GLM-5.1（x1.06）"},
				{Value: "glm-5.0", Label: "GLM-5.0（x0.80）"},
				{Value: "glm-5.0-turbo", Label: "GLM-5.0-Turbo（x0.95）"},
				{Value: "glm-5v-turbo", Label: "GLM-5v-Turbo（x0.95）"},
				{Value: "glm-4.7", Label: "GLM-4.7（x0.23）"},
				{Value: "minimax-m3", Label: "MiniMax-M3（x0.25）"},
				{Value: "minimax-m2.7", Label: "MiniMax-M2.7（x0.26）"},
				{Value: "kimi-k2.6", Label: "Kimi-K2.6（x0.59）"},
				{Value: "kimi-k2.5", Label: "Kimi-K2.5（x0.45）"},
				{Value: "hy3-preview", Label: "Hy3 preview（x0.37）"},
				{Value: "deepseek-v4-pro", Label: "Deepseek-V4-Pro（x0.25）"},
				{Value: "deepseek-v4-flash", Label: "Deepseek-V4-Flash（x0.13）"},
				{Value: "deepseek-v3-2-volc", Label: "DeepSeek-V3.2（x0.29）"},
			}},
		},
	}
}
