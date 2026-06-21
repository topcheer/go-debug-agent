package debugagent

// AgentConfig holds all configuration for the debug agent.
type AgentConfig struct {
	Enabled  bool
	BasePath string
	LLM      LLMConfig
}

// LLMConfig holds LLM provider settings.
type LLMConfig struct {
	BaseURL            string
	APIKey             string
	Model              string
	Temperature        float64
	MaxTokens          int
	MaxToolRounds      int
	TimeoutSeconds     int
	MaxRetries         int
	RetryBaseDelayMs   int
	RetryMaxDelayMs    int
	ContextWindowTokens int
}

// DefaultConfig returns a config populated from environment variables.
func DefaultConfig() *AgentConfig {
	return &AgentConfig{
		Enabled:  true,
		BasePath: "/agent",
		LLM: LLMConfig{
			BaseURL:            envOrDefault("LLM_BASE_URL", "https://open.bigmodel.cn/api/coding/paas/v4"),
			APIKey:             envOrDefault("LLM_API_KEY", envOrDefault("OPENAI_API_KEY", "")),
			Model:              envOrDefault("LLM_MODEL", "glm-5.2"),
			Temperature:        0.3,
			MaxTokens:          4096,
			MaxToolRounds:      25,
			TimeoutSeconds:     120,
			MaxRetries:         3,
			RetryBaseDelayMs:   1000,
			RetryMaxDelayMs:    30000,
			ContextWindowTokens: 100000,
		},
	}
}

func envOrDefault(key, defaultVal string) string {
	val := getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}
