package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Version 当前版本号
const Version = "0.4.0"

// Config 全局配置
type Config struct {
	LLM      LLMConfig
	Tools    ToolsConfig
	Memory   MemoryConfig
	TUI      TUIConfig
	Cron     CronConfig
	Telegram TelegramConfig

	// 全局选项
	Debug      bool
	ConfigPath string
}

// TelegramConfig Telegram Bot 配置
type TelegramConfig struct {
	BotToken    string
	AllowedChat int64 // 0 = 不限制
}

// LLMConfig LLM 相关配置
type LLMConfig struct {
	// OpenAI 兼容
	OpenAIAPIBase string
	OpenAIAPIKey  string
	OpenAIModel   string
	SmallModel    string
	Temperature   float64
	MaxTokens     int

	// Anthropic
	AnthropicAPIKey  string
	AnthropicAPIBase string

	// Ollama
	OllamaHost string

	// 通用
	MaxToolRounds   int
	MaxTurns        int
	CompleteTimeout int // 秒
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	DangerousCommands []string
	BashTimeout       int // 秒
	MaxOutputSize     int // 字节
	MaxWriteSize      int // 字节
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	DBPath     string
	MemoryFile string
	SessionDir string
	AuditLog   string
}

// TUIConfig TUI 配置
type TUIConfig struct {
	MaxSessions   int
	MaxInputChars int
}

// CronConfig 定时任务配置
type CronConfig struct {
	JobTimeout    int // 秒
	LogRetention  int
	MaxConcurrent int
}

// DefaultDangerousCommands 默认危险命令列表
var DefaultDangerousCommands = []string{
	"rm -rf /",
	"rm -rf ~",
	"dd if=",
	"mkfs",
	"> /dev/",
	":(){ :|:& };:",
}

// Load 加载配置（优先级: 硬编码默认 → DB → 环境变量）
func Load() *Config {
	// 第一步：硬编码默认值
	cfg := &Config{
		LLM: LLMConfig{
			OpenAIAPIBase:    "https://api.openai.com/v1",
			OpenAIModel:      "gpt-4o",
			Temperature:      0.7,
			MaxTokens:        4096,
			AnthropicAPIBase: "https://api.anthropic.com",
			OllamaHost:       "http://localhost:11434",
			MaxToolRounds:    10,
			MaxTurns:         20,
			CompleteTimeout:  8,
		},
		Tools: ToolsConfig{
			DangerousCommands: DefaultDangerousCommands,
			BashTimeout:       60,
			MaxOutputSize:     51200,
			MaxWriteSize:      1048576,
		},
		Memory: MemoryConfig{
			DBPath:     getEnv("KELE_DB_PATH", filepath.Join(keleDir(), "memory.db")),
			MemoryFile: getEnv("KELE_MEMORY_FILE", filepath.Join(keleDir(), "MEMORY.md")),
			SessionDir: getEnv("KELE_SESSION_DIR", filepath.Join(keleDir(), "sessions")),
			AuditLog:   getEnv("KELE_AUDIT_LOG", filepath.Join(keleDir(), "audit.log")),
		},
		TUI: TUIConfig{
			MaxSessions:   9,
			MaxInputChars: 5000,
		},
		Cron: CronConfig{
			JobTimeout:    300,
			LogRetention:  50,
			MaxConcurrent: 5,
		},
	}

	// 第二步：DB 覆盖（基准配置）
	ApplyToConfig(cfg)

	// 第三步：环境变量覆盖（最高优先级）
	applyEnvOverrides(cfg)

	return cfg
}

// applyEnvOverrides 环境变量覆盖（最高优先级）
func applyEnvOverrides(cfg *Config) {
	// LLM
	if v := os.Getenv("OPENAI_API_BASE"); v != "" {
		cfg.LLM.OpenAIAPIBase = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.LLM.OpenAIAPIKey = v
	}
	if v := os.Getenv("OPENAI_MODEL"); v != "" {
		cfg.LLM.OpenAIModel = v
	}
	if v := os.Getenv("KELE_SMALL_MODEL"); v != "" {
		cfg.LLM.SmallModel = v
	}
	if v := os.Getenv("KELE_TEMPERATURE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.LLM.Temperature = f
		}
	}
	if v := os.Getenv("KELE_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.LLM.MaxTokens = n
		}
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.LLM.AnthropicAPIKey = v
	}
	if v := os.Getenv("ANTHROPIC_API_BASE"); v != "" {
		cfg.LLM.AnthropicAPIBase = v
	}
	if v := os.Getenv("OLLAMA_HOST"); v != "" {
		cfg.LLM.OllamaHost = v
	}
	if v := os.Getenv("KELE_MAX_TOOL_ROUNDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.LLM.MaxToolRounds = n
		}
	}
	if v := os.Getenv("KELE_MAX_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.LLM.MaxTurns = n
		}
	}
	if v := os.Getenv("KELE_COMPLETE_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.LLM.CompleteTimeout = n
		}
	}

	// Tools
	if v := os.Getenv("KELE_BASH_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Tools.BashTimeout = n
		}
	}
	if v := os.Getenv("KELE_MAX_OUTPUT_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Tools.MaxOutputSize = n
		}
	}
	if v := os.Getenv("KELE_MAX_WRITE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Tools.MaxWriteSize = n
		}
	}

	// TUI
	if v := os.Getenv("KELE_MAX_SESSIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.TUI.MaxSessions = n
		}
	}
	if v := os.Getenv("KELE_MAX_INPUT_CHARS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.TUI.MaxInputChars = n
		}
	}

	// Cron
	if v := os.Getenv("KELE_CRON_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Cron.JobTimeout = n
		}
	}
	if v := os.Getenv("KELE_CRON_LOG_RETENTION"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Cron.LogRetention = n
		}
	}
	if v := os.Getenv("KELE_CRON_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Cron.MaxConcurrent = n
		}
	}
}

// ApplyFlags 应用 CLI 参数覆盖
func (c *Config) ApplyFlags(model string, debug bool, configPath string) {
	if model != "" {
		c.LLM.OpenAIModel = model
	}
	c.Debug = debug
	if configPath != "" {
		c.ConfigPath = configPath
	}
}

// IsDangerous 检查命令是否危险
func (c *Config) IsDangerous(command string) bool {
	lower := strings.ToLower(command)
	for _, d := range c.Tools.DangerousCommands {
		if strings.Contains(lower, strings.ToLower(d)) {
			return true
		}
	}
	return false
}

// HasOpenAI 检查是否配置了 OpenAI
func (c *Config) HasOpenAI() bool {
	return c.LLM.OpenAIAPIKey != ""
}

// HasAnthropic 检查是否配置了 Anthropic
func (c *Config) HasAnthropic() bool {
	return c.LLM.AnthropicAPIKey != ""
}

// --- 辅助函数 ---

// keleDir 返回 ~/.kele 绝对路径
func keleDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".kele")
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
