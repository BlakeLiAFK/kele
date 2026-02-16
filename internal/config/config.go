package config

import (
	"os"
	"strconv"
	"strings"
)

// Version 当前版本号
const Version = "0.2.0"

// Config 全局配置
type Config struct {
	LLM    LLMConfig
	Tools  ToolsConfig
	Memory MemoryConfig
	TUI    TUIConfig
	Cron   CronConfig
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

// Load 加载配置（环境变量 > 默认值）
func Load() *Config {
	cfg := &Config{
		LLM: LLMConfig{
			OpenAIAPIBase:    getEnv("OPENAI_API_BASE", "https://api.openai.com/v1"),
			OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
			OpenAIModel:      getEnv("OPENAI_MODEL", "gpt-4o"),
			SmallModel:       os.Getenv("KELE_SMALL_MODEL"),
			Temperature:      getEnvFloat("KELE_TEMPERATURE", 0.7),
			MaxTokens:        getEnvInt("KELE_MAX_TOKENS", 4096),
			AnthropicAPIKey:  os.Getenv("ANTHROPIC_API_KEY"),
			AnthropicAPIBase: getEnv("ANTHROPIC_API_BASE", "https://api.anthropic.com"),
			OllamaHost:       getEnv("OLLAMA_HOST", "http://localhost:11434"),
			MaxToolRounds:    getEnvInt("KELE_MAX_TOOL_ROUNDS", 10),
			MaxTurns:         getEnvInt("KELE_MAX_TURNS", 20),
			CompleteTimeout:  getEnvInt("KELE_COMPLETE_TIMEOUT", 8),
		},
		Tools: ToolsConfig{
			DangerousCommands: DefaultDangerousCommands,
			BashTimeout:       getEnvInt("KELE_BASH_TIMEOUT", 60),
			MaxOutputSize:     getEnvInt("KELE_MAX_OUTPUT_SIZE", 51200),
			MaxWriteSize:      getEnvInt("KELE_MAX_WRITE_SIZE", 1048576),
		},
		Memory: MemoryConfig{
			DBPath:     getEnv("KELE_DB_PATH", ".kele/memory.db"),
			MemoryFile: getEnv("KELE_MEMORY_FILE", ".kele/MEMORY.md"),
			SessionDir: getEnv("KELE_SESSION_DIR", ".kele/sessions"),
		},
		TUI: TUIConfig{
			MaxSessions:   getEnvInt("KELE_MAX_SESSIONS", 9),
			MaxInputChars: getEnvInt("KELE_MAX_INPUT_CHARS", 5000),
		},
		Cron: CronConfig{
			JobTimeout:    getEnvInt("KELE_CRON_TIMEOUT", 300),
			LogRetention:  getEnvInt("KELE_CRON_LOG_RETENTION", 50),
			MaxConcurrent: getEnvInt("KELE_CRON_MAX_CONCURRENT", 5),
		},
	}

	// CLI flags 覆盖（由 main.go 调用 ApplyFlags 设置）
	return cfg
}

// ApplyFlags 应用 CLI 参数覆盖
func (c *Config) ApplyFlags(model string, debug bool) {
	if model != "" {
		c.LLM.OpenAIModel = model
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

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultVal
}
