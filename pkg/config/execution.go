package config

import (
	"fmt"
	"strings"
)

// ExecutionConfig Execution 模块运行参数（含执行代理）。
type ExecutionConfig struct {
	AgentRegisterToken string `mapstructure:"agent_register_token"`
	LeaseTTLSeconds    int    `mapstructure:"lease_ttl_seconds"`
}

var weakAgentRegisterTokens = []string{
	"dev-agent-register-token",
	"agent-register-token",
	"changeme",
	"change-me",
	"secret",
	"password",
}

func validateExecutionConfig(cfg ExecutionConfig, env string) error {
	if env != "prod" {
		return nil
	}
	return ValidateAgentRegisterToken(cfg.AgentRegisterToken, env)
}

// ValidateAgentRegisterToken 校验执行代理注册令牌；prod 必填且须达到与 JWT 相近的熵要求。
func ValidateAgentRegisterToken(token, env string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		if env == "prod" {
			return fmt.Errorf("execution.agent_register_token must be set in prod")
		}
		return nil
	}
	if env == "dev" || env == "test" {
		return nil
	}
	normalized := strings.ToLower(token)
	for _, weak := range weakAgentRegisterTokens {
		if normalized == strings.ToLower(weak) {
			return fmt.Errorf("execution.agent_register_token is a known weak or placeholder value (env=%q)", env)
		}
	}
	if len(token) < minJWTSecretLenNonDev {
		return fmt.Errorf("execution.agent_register_token too short (>=32 bytes required for env=%q)", env)
	}
	if uniqueRunes(token) < minJWTSecretUnique {
		return fmt.Errorf("execution.agent_register_token lacks character diversity (env=%q)", env)
	}
	if maxRepeatRun(token) > maxJWTSecretRepeatRun {
		return fmt.Errorf("execution.agent_register_token contains excessive repeated characters (env=%q)", env)
	}
	entropy := shannonEntropy(token)
	classes := countCharClasses(token)
	if classes < 3 && entropy < minJWTSecretEntropy+0.7 {
		return fmt.Errorf("execution.agent_register_token must include more character classes or higher entropy (env=%q)", env)
	}
	if entropy < minJWTSecretEntropy {
		return fmt.Errorf("execution.agent_register_token entropy too low (env=%q)", env)
	}
	return nil
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
