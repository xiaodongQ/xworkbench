package config

import (
	"encoding/json"
	"fmt"
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

	// AI Chat 配置（模型 API + 函数调用）
	AIChat AIChatConfig `json:"ai_chat"`

	// Scheduler 调度器配置
	Scheduler SchedulerConfig `json:"scheduler"`
}

// SchedulerConfig 调度器相关配置
type SchedulerConfig struct {
	MaxResumeCount int `json:"max_resume_count"` // AI 任务最大连续 resume 次数，默认 20 次后重置会话
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
	Bin       string   `json:"bin"`   // 可执行文件名或路径
	Args      []string `json:"args"`  // 本地唤起参数，{dir} 会被替换为目录路径
	Name      string   `json:"name"`  // 显示名称
	Plate     string   `json:"plate"` // 适用平台：all / macOS / Windows / Linux
	Path      string   `json:"path"`  // 检测到的自定义路径
	RemoteBin string   `json:"remote_bin,omitempty"` // 远程唤起的 ssh 路径（默认 "ssh"）
	RemoteArgs []string `json:"remote_args,omitempty"` // 远程唤起参数模板
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

// ProviderConfig 单个 AI Provider 的配置（API Key 不写入日志）。
type ProviderConfig struct {
	APIKey      string  `json:"api_key,omitempty"`
	BaseURL     string  `json:"base_url,omitempty"`
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"` // 0.0-2.0，默认 0.7
	MaxTokens   int     `json:"max_tokens,omitempty"` // 默认 4096
}

// MaskedAPIKey returns a masked version of the API key (first 4 + last 4 chars visible, middle masked).
func (p ProviderConfig) MaskedAPIKey() string {
	if len(p.APIKey) <= 8 {
		return "sk-" + strings.Repeat("•", max(0, len(p.APIKey)-3))
	}
	return p.APIKey[:4] + strings.Repeat("•", len(p.APIKey)-8) + p.APIKey[len(p.APIKey)-4:]
}

// AIChatConfig AI Chat 对话配置（Anthropic / OpenAI 双 Provider 共存）。
type AIChatConfig struct {
	ActiveProvider string         `json:"active_provider,omitempty"` // "openai" | "anthropic"，空=默认 anthropic
	Anthropic     ProviderConfig `json:"anthropic"`
	OpenAI        ProviderConfig `json:"openai"`
	SystemPrompt  string         `json:"system_prompt,omitempty"`
}

// IsEnabled returns true when at least one provider has both api_key and model.
func (c AIChatConfig) IsEnabled() bool {
	return (c.Anthropic.APIKey != "" && c.Anthropic.Model != "") ||
		(c.OpenAI.APIKey != "" && c.OpenAI.Model != "")
}

// GetActive returns the currently active ProviderConfig.
func (c AIChatConfig) GetActive() ProviderConfig {
	switch c.ActiveProvider {
	case "openai":
		return c.OpenAI
	default:
		return c.Anthropic
	}
}

// MaskedAPIKey returns a masked version of the active provider's API key.
func (c AIChatConfig) MaskedAPIKey() string {
	return c.GetActive().MaskedAPIKey()
}
// Save 将当前 AppConfig 写回 config.json。
// 兼容策略：读取原文件，只覆盖 Config 结构体认识的字段，保留文件中其他未知字段
//（如下次升级新增的字段、用户手动添加的字段等）。
func Save() error {
	AppConfigMu.RLock()
	current := AppConfig
	AppConfigMu.RUnlock()
	configFilePathMu.RLock()
	path := configFilePath
	configFilePathMu.RUnlock()
	if current == nil {
		return nil
	}
	// 优先用已知路径，找不到时降级到可执行文件同目录的 data/config.json
	if path == "" {
		path = configPath()
	}
	if path == "" {
		// 完全没有配置文件时，创建默认路径（可执行文件同目录/data/config.json）
		// 确保 Save 不会静默失败
		exe, err := os.Executable()
		if err == nil && exe != "" {
			dir := filepath.Dir(exe)
			path = filepath.Join(dir, "data", "config.json")
		}
	}
	if path == "" {
		return fmt.Errorf("config save: no config path available")
	}

	// 1. 用 DefaultConfig 补全当前配置缺失的字段（零值字段用默认值填充），
	//    这样 Save 时新字段不会被 omitempty 吞掉，能写到文件里。
	defaults := DefaultConfig()
	filled := fillDefaults(current, defaults)

	// 2. 把补全后的 Config marshal 成 map
	currentMap, err := toStringMap(filled)
	if err != nil {
		return err
	}

	// 2. 尝试读取原文件，保留其中 Config 不认识的字段
	var merged map[string]interface{}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		var existing map[string]interface{}
		if err := json.Unmarshal(raw, &existing); err == nil {
			merged = mergeMapPreserve(existing, currentMap)
		} else {
			merged = currentMap
		}
	} else {
		merged = currentMap
	}

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// toStringMap 将 Config 结构体转为 map（omitempty 语义：零值字段不出现）。
func toStringMap(c *Config) (map[string]interface{}, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// mergeMapPreserve 将 current 合并到 base 中：current 有值的字段覆盖 base，base 中
// 有但 current 没有（且不为零）的字段保留，以此实现"保留未知字段"。
func mergeMapPreserve(base, current map[string]interface{}) map[string]interface{} {
	if base == nil {
		return current
	}
	out := shallowClone(base)
	for k, v := range current {
		out[k] = v
	}
	return out
}

func shallowClone(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	r := make(map[string]interface{}, len(m))
	for k, v := range m {
		r[k] = v
	}
	return r
}

func Load() (*Config, error) {
	cfg := DefaultConfig()
	path := configPath()
	if path == "" {
		return cfg, nil
	}
	loaded, err := loadFromFile(path, cfg)
	if err != nil {
		return loaded, err
	}
	configFilePathMu.Lock()
	configFilePath = path
	configFilePathMu.Unlock()
	return loaded, nil
}

// LoadFromPath 从指定路径加载配置，覆盖全局 AppConfig（线程安全）
// 目标文件不存在时，自动从模板（可执行文件同目录的 config.template.conf）创建。
func LoadFromPath(path string) error {
	if path == "" {
		return nil
	}
	cfg := DefaultConfig()
	loaded, err := loadFromFile(path, cfg)
	if err != nil && os.IsNotExist(err) {
		// 文件不存在，尝试从模板创建
		if err := ensureConfigFromTemplate(path); err != nil {
			return fmt.Errorf("config file not found and template copy failed: %v", err)
		}
		loaded, err = loadFromFile(path, cfg)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	Set(loaded) // 走锁
	configFilePathMu.Lock()
	configFilePath = path
	configFilePathMu.Unlock()
	return nil
}

// ensureConfigFromTemplate 尝试从可执行文件同目录的 config.template.conf 拷贝到目标路径。
func ensureConfigFromTemplate(targetPath string) error {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return fmt.Errorf("cannot determine executable path")
	}
	dir := filepath.Dir(exe)
	tplPath := filepath.Join(dir, "config.template.conf")
	data, err := os.ReadFile(tplPath)
	if err != nil {
		return fmt.Errorf("template not found: %s", tplPath)
	}
	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("create config dir failed: %v", err)
	}
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("write config failed: %v", err)
	}
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

	// ai_chat（只合并非零字段，保留 dst 原有的每一项）
	if src.AIChat.ActiveProvider != "" {
		dst.AIChat.ActiveProvider = src.AIChat.ActiveProvider
	}
	// Anthropic
	if src.AIChat.Anthropic.APIKey != "" {
		dst.AIChat.Anthropic.APIKey = src.AIChat.Anthropic.APIKey
	}
	if src.AIChat.Anthropic.BaseURL != "" {
		dst.AIChat.Anthropic.BaseURL = src.AIChat.Anthropic.BaseURL
	}
	if src.AIChat.Anthropic.Model != "" {
		dst.AIChat.Anthropic.Model = src.AIChat.Anthropic.Model
	}
	if src.AIChat.Anthropic.Temperature > 0 {
		dst.AIChat.Anthropic.Temperature = src.AIChat.Anthropic.Temperature
	}
	if src.AIChat.Anthropic.MaxTokens > 0 {
		dst.AIChat.Anthropic.MaxTokens = src.AIChat.Anthropic.MaxTokens
	}
	// OpenAI
	if src.AIChat.OpenAI.APIKey != "" {
		dst.AIChat.OpenAI.APIKey = src.AIChat.OpenAI.APIKey
	}
	if src.AIChat.OpenAI.BaseURL != "" {
		dst.AIChat.OpenAI.BaseURL = src.AIChat.OpenAI.BaseURL
	}
	if src.AIChat.OpenAI.Model != "" {
		dst.AIChat.OpenAI.Model = src.AIChat.OpenAI.Model
	}
	if src.AIChat.OpenAI.Temperature > 0 {
		dst.AIChat.OpenAI.Temperature = src.AIChat.OpenAI.Temperature
	}
	if src.AIChat.OpenAI.MaxTokens > 0 {
		dst.AIChat.OpenAI.MaxTokens = src.AIChat.OpenAI.MaxTokens
	}
}

// fillDefaults 把 defaults 的默认值填充到 current 中缺失或零值的字段，
// 返回填充后的深拷贝（不修改 current）。用于 Save() 时补全新版本新增的字段，
// 避免 omitempty 把零值字段吞掉导致历史配置文件丢字段。
func fillDefaults(current, defaults *Config) *Config {
	if defaults == nil {
		return current
	}
	if current == nil {
		return defaults
	}
	currentMap, _ := toStringMap(current)
	defaultsMap, _ := toStringMap(defaults)
	merged := fillDefaultsRecursive(defaultsMap, currentMap)

	var out Config
	data, _ := json.Marshal(merged)
	json.Unmarshal(data, &out)
	return &out
}

// fillDefaultsRecursive 递归合并两个 map：base 有值则用 base，否则用 defaults。
func fillDefaultsRecursive(defaults, current map[string]interface{}) map[string]interface{} {
	if defaults == nil {
		return current
	}
	out := shallowClone(defaults)
	for k, v := range current {
		if v == nil {
			continue
		}
		// 两者都是 map 时递归合并
		defMap, defIsMap := defaults[k].(map[string]interface{})
		curMap, curIsMap := v.(map[string]interface{})
		if defIsMap && curIsMap {
			out[k] = fillDefaultsRecursive(defMap, curMap)
		} else {
			out[k] = v
		}
	}
	return out
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
	// 优先找 data/config.json（可迁移、可复用）
	dataPath := filepath.Join(dir, "data", "config.json")
	if _, err := os.Stat(dataPath); err == nil {
		return dataPath
	}
	// 降级：找可执行文件同目录的 config.json（旧部署兼容）
	legacyPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(legacyPath); err == nil {
		fmt.Fprintf(os.Stderr, "[xworkbench config] WARNING: using legacy config path (./config.json); "+
			"please move it to data/config.json for easier project migration.\n")
		return legacyPath
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
		Scheduler: SchedulerConfig{
			MaxResumeCount: 20,
		},
		Terminal: TerminalConfig{
			DetectPaths: map[string][]string{
				"wezterm": {"/Applications/WezTerm.app/Contents/MacOS/WezTerm"},
			},
			Types: map[string]TerminalTypeDef{
				"wezterm": {
					Bin:       "wezterm",
					Args:      []string{"start", "--cwd", "{dir}", "--always-new-process"},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}", "-t", "--", "sh", "-c", "{shell_cmd}"},
					Name:  "WezTerm",
					Plate: "all",
					Path: "",
				},
				"wt": {
					Bin:       "wt.exe",
					Args:      []string{"-d", "{dir}"},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}"},
					Name:  "Windows Terminal",
					Plate: "windows",
					Path: "",
				},
				"powershell": {
					Bin:       "powershell.exe",
					Args:      []string{"-NoExit", "-Command", "cd \"{dir}\""},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}"},
					Name:  "PowerShell",
					Plate: "windows",
					Path: "",
				},
				"pwsh": {
					Bin:       "pwsh.exe",
					Args:      []string{"-NoExit", "-Command", "cd \"{dir}\""},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}"},
					Name:  "PowerShell Core",
					Plate: "windows",
					Path: "",
				},
				"pwsh7": {
					Bin:       "pwsh",
					Args:      []string{"-NoExit", "-c", "cd '{dir}'"},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}"},
					Name:  "PowerShell 7",
					Plate: "all",
					Path: "",
				},
				"iterm2": {
					Bin:       "osascript",
					Args:      []string{"-e", `tell application "iTerm2" to create window with default profile`},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}"},
					Name:  "iTerm2",
					Plate: "darwin",
					Path: "",
				},
				"terminal": {
					Bin:       "osascript",
					Args:      []string{"-e", `tell application "Terminal" to do script "cd {dir}"`},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}"},
					Name:  "Terminal.app",
					Plate: "darwin",
					Path: "",
				},
				"gnome": {
					Bin:       "gnome-terminal",
					Args:      []string{"--", "--working-directory={dir}"},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}"},
					Name:  "GNOME Terminal",
					Plate: "linux",
					Path: "",
				},
				"xterm": {
					Bin:       "xterm",
					Args:      []string{"-e", "bash -c 'cd {dir}; exec bash'"},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}"},
					Name:  "xterm",
					Plate: "linux",
					Path: "",
				},
				"cmd": {
					Bin:       "cmd.exe",
					Args:      []string{"/K", "cd /d {dir}"},
					RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}"},
					Name:  "cmd.exe",
					Plate: "windows",
					Path: "",
				},
			},
		},
		Relay: RelayConfig{
			APIKey: "xworkbench",
		},
		AIChat: AIChatConfig{
			ActiveProvider: "anthropic",
			Anthropic: ProviderConfig{
				BaseURL:     "https://api.anthropic.com",
				Model:       "claude-3-5-sonnet-20241022",
				Temperature: 0.7,
				MaxTokens:   4096,
			},
			OpenAI: ProviderConfig{
				BaseURL:     "https://api.openai.com/v1",
				Model:       "gpt-4o",
				Temperature: 0.7,
				MaxTokens:   4096,
			},
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