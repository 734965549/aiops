package config

// ExecutionConfig Execution 模块运行参数（含执行代理）。
type ExecutionConfig struct {
	AgentRegisterToken string `mapstructure:"agent_register_token"`
	LeaseTTLSeconds    int    `mapstructure:"lease_ttl_seconds"`
}

func (c ExecutionConfig) LeaseTTLSecondsOrDefault() int {
	if c.LeaseTTLSeconds <= 0 {
		return 300
	}
	return c.LeaseTTLSeconds
}

func (c ExecutionConfig) AgentRegisterTokenOrDefault(env string) string {
	if c.AgentRegisterToken != "" {
		return c.AgentRegisterToken
	}
	if env == "dev" || env == "test" {
		return "dev-agent-register-token"
	}
	return ""
}
