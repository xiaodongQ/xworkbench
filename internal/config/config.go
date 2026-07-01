package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// AppConfig 全局应用配置指针（向后兼容 + 测试 setup 用）。
//
// ⚠️ 直接读写 AppConfig.X 字段在生产代码里不安全（HTTP handler 与后台
// goroutine 并发）。生产代码应走 Snapshot()/Get()/Update()/SetAndSave()
// 之一，路径内部已加锁 + copy-on-write。
//
// 测试代码（同一进程顺序执行、不会并发）可以直接用 AppConfig 设初值。
var (
	AppConfigMu sync.RWMutex
	AppConfig   *Config
)

// Get 返回当前配置原指针（线程安全）。调用方应在锁内只读访问，**不要**
// 修改返回值字段——会绕过锁。需要修改请用 Update/SetAndSave。
func Get() *Config {
	AppConfigMu.RLock()
	defer AppConfigMu.RUnlock()
	return AppConfig
}

// Set 整体替换配置（线程安全）。写路径在 init/Load/LoadFromPath/Update 等场景用。
func Set(c *Config) {
	AppConfigMu.Lock()
	defer AppConfigMu.Unlock()
	AppConfig = c
}

// Snapshot 返回当前配置的深拷贝。调用方修改返回值不会影响全局，适合
// 一次性"读多字段"或"读后加工"的场景。
func Snapshot() *Config {
	AppConfigMu.RLock()
	defer AppConfigMu.RUnlock()
	if AppConfig == nil {
		return nil
	}
	cp := *AppConfig
	return &cp
}

// Update 以 copy-on-write 方式修改配置：fn 收到当前快照副本，fn 修改完后
// 整个替换 AppConfig，期间其它 goroutine 通过 Snapshot/Get 始终看到一致状态。
func Update(fn func(c *Config)) *Config {
	AppConfigMu.Lock()
	defer AppConfigMu.Unlock()
	if AppConfig == nil {
		AppConfig = DefaultConfig()
	}
	cp := *AppConfig
	fn(&cp)
	AppConfig = &cp
	return AppConfig
}

// SetAndSave 是 handleSetConfig 等"改完要落盘"场景的安全封装：
//
//  1. copy-on-write 修改内存（不会半改状态被读到）
//  2. Save() 落盘
//  3. 若 Save 失败：回滚内存（拷贝原始快照，原样写回 AppConfig）
//
// 返回最终生效的 *Config + Save 错误。调用方应在 Save 失败时把内存回滚
// 状态告知用户（API 500）。
func SetAndSave(fn func(c *Config)) (*Config, error) {
	AppConfigMu.Lock()
	original := AppConfig
	if original == nil {
		original = DefaultConfig()
	}
	cp := *original
	fn(&cp)
	AppConfig = &cp
	AppConfigMu.Unlock()

	if err := Save(); err != nil {
		// Save 失败：内存回滚到 original
		AppConfigMu.Lock()
		AppConfig = original
		AppConfigMu.Unlock()
		return original, err
	}
	return AppConfig, nil
}

// TestSnapshotAndRestore 返回一个 restore 函数，调用时把 AppConfig 恢复到
// 调用时刻的状态（深拷贝）。用法：
//
//	func TestXxx(t *testing.T) {
//	    t.Cleanup(config.TestSnapshotAndRestore())
//	    config.Update(...) 或 config.Set(...)
//	}
//
// 这比 `orig := config.AppConfig; defer config.AppConfig = orig` 更安全：
// 后者只恢复指针不恢复字段内容（helper 中途修改过就翻车）。
func TestSnapshotAndRestore() func() {
	snap := Snapshot()
	return func() {
		if snap == nil {
			return
		}
		Set(snap)
	}
}

// configFilePath 记录最近一次加载的配置文件路径，供 Save() 使用
var (
	configFilePathMu sync.RWMutex
	configFilePath   string
)

// Config 全局应用配置（单一来源：config.json）
//
// 顶层字段为用户偏好（default_terminal / preferred_cli / ai_loop_enabled /
// aichat_default_cli / todo_md_path / scheduler_enabled），改完 Save() 立即落盘。
// nested 结构（terminal / models / relay）为部署级配置，类型/模型/认证密钥。
type Config struct {
	// 用户偏好（顶层，单一来源；空值即未设）
	DefaultTerminal  string `json:"default_terminal,omitempty"`
	PreferredCLI     string `json:"preferred_cli,omitempty"` // claude | cbc；空=默认 claude
	AILoopEnabled    bool   `json:"ai_loop_enabled"`
	AichatDefaultCLI string `json:"aichat_default_cli,omitempty"` // codex/cbc/shell/claude
	DangerouslySkipPermissions bool `json:"dangerously_skip_permissions"` // 完全放开 CLI 权限：跳过 --allowedTools、改为 --dangerously-skip-permissions；默认 false，开启后 AI 可执行任意命令
	TodoMDPath       string `json:"todo_md_path,omitempty"`
	SchedulerEnabled bool   `json:"scheduler_enabled"`

	// 部署级配置
	Relay    RelayConfig    `json:"relay"`
	Terminal TerminalConfig `json:"terminal"`
	Models   ModelsConfig   `json:"models"`
	SSH      SSHKeyConfig   `json:"ssh"`
}

// SupportedCLIs 允许的 CLI 名（不区分大小写）
var SupportedCLIs = map[string]bool{
	"claude": true,
	"cbc":    true,
}

// NormalizeCLI 归一化（trim + 小写）；空返 ""
func NormalizeCLI(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return s
}

// IsValidCLI 是否为支持的 CLI
func IsValidCLI(s string) bool {
	return SupportedCLIs[NormalizeCLI(s)]
}

type RelayConfig struct {
	APIKey string `json:"api_key"` // 空=关闭认证，非空=Bearer token（X-API-Key 头也支持）
}

type TerminalConfig struct {
	DetectPaths map[string][]string        `json:"detect_paths"` // 检测路径列表
	Types       map[string]TerminalTypeDef `json:"types"`        // 终端类型定义
}

// SSHKeyConfig SSH 密钥相关全局配置
type SSHKeyConfig struct {
	DefaultKeyPath string `json:"default_key_path,omitempty"` // 全局默认私钥路径（单记录可覆盖）
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
	Default     string        `json:"default"`               // 任务执行默认模型
	EvalDefault string        `json:"eval_default,omitempty"` // 评估默认模型（未设则 fallback 到 Default）
	Options     []ModelOption `json:"options"`
}

type ModelOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Load 加载配置（自动找路径或用默认）
// Save 将当前 AppConfig 写回 config.json
func Save() error {
	// 读 AppConfig 用 RLock。Marshal 期间我们仍持有 RLock，其它 goroutine
	// 可以继续读但不能写——保证序列化过程中 Config 字段不变。
	AppConfigMu.RLock()
	current := AppConfig
	AppConfigMu.RUnlock()
	configFilePathMu.RLock()
	path := configFilePath
	configFilePathMu.RUnlock()
	if current == nil {
		return nil
	}
	if path == "" {
		path = configPath()
	}
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(current, "", "  ")
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

// LoadFromPath 从指定路径加载配置，覆盖全局 AppConfig（线程安全）
func LoadFromPath(path string) error {
	if path == "" {
		return nil
	}
	cfg := DefaultConfig()
	loaded, err := loadFromFile(path, cfg)
	if err != nil {
		return err
	}
	Set(loaded) // 走锁
	configFilePathMu.Lock()
	configFilePath = path
	configFilePathMu.Unlock()
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

// mergeConfig 把 src 合并到 dst；src 中零值字段不动 dst。
// 注意：bool 字段（AILoopEnabled / SchedulerEnabled）必须显式写入，
// 因为 false 也是合法值，不能用"src == 零值"判定"未设"。
func mergeConfig(dst, src *Config) {
	// 顶层用户偏好（字符串字段：空=未设，跳过合并）
	if src.DefaultTerminal != "" {
		dst.DefaultTerminal = src.DefaultTerminal
	}
	if src.PreferredCLI != "" {
		dst.PreferredCLI = src.PreferredCLI
	}
	if src.AichatDefaultCLI != "" {
		dst.AichatDefaultCLI = src.AichatDefaultCLI
	}
	if src.TodoMDPath != "" {
		dst.TodoMDPath = src.TodoMDPath
	}
	// bool 字段：直接覆盖（false 也是合法值）
	dst.AILoopEnabled = src.AILoopEnabled
	dst.SchedulerEnabled = src.SchedulerEnabled

	// terminal（无 default_type，仅 types + detect_paths）
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
	// relay（直接赋值，空字符串也可覆盖默认值）
	dst.Relay.APIKey = src.Relay.APIKey
	if dst.Relay.APIKey == "" {
		dst.Relay.APIKey = "xworkbench"
	}

	// models
	for cliType, srcGroup := range src.Models {
		dstGroup := dst.Models[cliType]
		if srcGroup.Default != "" {
			dstGroup.Default = srcGroup.Default
		}
		// eval_default 留空表示"未设"，merge 时跳过（保留 dst 的值或留空）
		if srcGroup.EvalDefault != "" {
			dstGroup.EvalDefault = srcGroup.EvalDefault
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
		DefaultTerminal:  "wezterm",
		PreferredCLI:     "claude",
		AichatDefaultCLI: "claude",
		DangerouslySkipPermissions: false,
		Terminal: TerminalConfig{
			DetectPaths: map[string][]string{
				"wezterm": {"/Applications/WezTerm.app/Contents/MacOS/WezTerm"},
			},
			Types: map[string]TerminalTypeDef{
				"wezterm":    {Bin: "wezterm", Args: []string{"start", "--cwd", "{dir}", "--always-new-process"}, Name: "WezTerm", Plate: "all", Path: ""},
				"wt":         {Bin: "wt.exe", Args: []string{"-d", "{dir}"}, Name: "Windows Terminal", Plate: "windows", Path: ""},
				"powershell": {Bin: "powershell.exe", Args: []string{"-NoExit", "-Command", "cd \"{dir}\""}, Name: "PowerShell", Plate: "windows", Path: ""},
				"pwsh":       {Bin: "pwsh.exe", Args: []string{"-NoExit", "-Command", "cd \"{dir}\""}, Name: "PowerShell Core", Plate: "windows", Path: ""},
				"pwsh7":      {Bin: "pwsh", Args: []string{"-NoExit", "-c", "cd '{dir}'"}, Name: "PowerShell 7", Plate: "all", Path: ""},
				"terminal":   {Bin: "osascript", Args: []string{"-e", `tell application "Terminal" to do script "cd {dir}"`}, Name: "Terminal.app", Plate: "darwin", Path: ""},
				"gnome":      {Bin: "gnome-terminal", Args: []string{"--", "--working-directory={dir}"}, Name: "GNOME Terminal", Plate: "linux", Path: ""},
				"xterm":      {Bin: "xterm", Args: []string{"-e", "bash -c 'cd {dir}; exec bash'"}, Name: "xterm", Plate: "linux", Path: ""},
				"cmd":        {Bin: "cmd.exe", Args: []string{"/K", "cd /d {dir}"}, Name: "cmd.exe", Plate: "windows", Path: ""},
			},
		},
		Relay: RelayConfig{
			APIKey: "xworkbench",
		},
		Models: ModelsConfig{
			"claude": {Default: "sonnet", EvalDefault: "haiku", Options: []ModelOption{
				{Value: "haiku", Label: "haiku（快+便宜）"},
				{Value: "sonnet", Label: "sonnet（推荐 · 准确）"},
				{Value: "opus", Label: "opus（最强 · 贵）"},
			}},
			"cbc": {Default: "glm-5.1", EvalDefault: "glm-5.0", Options: []ModelOption{
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