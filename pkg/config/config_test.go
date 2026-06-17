package config

import "testing"

func TestConfigValidateAIExample(t *testing.T) {
	c := &Config{
		App:      AppConfig{Env: "dev"},
		Server:   ServerConfig{Port: 8080},
		Database: DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Auth:     AuthConfig{JWTSecret: DefaultJWTSecretPlaceholder},
		AI:       AIConfig{Providers: []AIProviderConfig{{ID: "demo-http-a", Name: "Demo HTTP Provider A", Type: "a", BaseURL: "http://127.0.0.1:9000", APIKey: "demo-api-key", TimeoutMS: 30000, Enabled: true}}},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected validate success, got %v", err)
	}
	if len(c.AI.Providers) != 1 || c.AI.Providers[0].Type != "a" {
		t.Fatalf("unexpected ai config: %+v", c.AI)
	}
}

func TestConfigValidateRejectsBadAIExample(t *testing.T) {
	c := &Config{
		App:      AppConfig{Env: "prod"},
		Server:   ServerConfig{Port: 8080},
		Database: DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Auth:     AuthConfig{JWTSecret: DefaultJWTSecretPlaceholder},
	}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected validate error for prod default jwt secret")
	}
}

func TestConfigValidateRequiresRedisInProd(t *testing.T) {
	c := &Config{
		App:      AppConfig{Env: "prod"},
		Server:   ServerConfig{Port: 8080},
		Database: DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Redis:    RedisConfig{Required: false},
		Auth:     AuthConfig{JWTSecret: "this-is-a-strong-enough-secret-for-prod-test-32b"},
		CORS:     CORSConfig{AllowOrigins: []string{"https://app.example.com"}},
	}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected validate error when redis.required is false in prod")
	}
	c.Redis.Required = true
	if err := c.Validate(); err != nil {
		t.Fatalf("expected validate success with redis.required in prod, got %v", err)
	}
}

func TestConfigNormalizeLoginIPAllowlist(t *testing.T) {
	c := &Config{Auth: AuthConfig{LoginIPAllowlist: []string{"192.0.2.1, 198.51.100.0/24"}}}
	c.normalize()
	if len(c.Auth.LoginIPAllowlist) != 2 {
		t.Fatalf("unexpected allowlist: %+v", c.Auth.LoginIPAllowlist)
	}
	if c.Auth.LoginIPAllowlist[0] != "192.0.2.1" || c.Auth.LoginIPAllowlist[1] != "198.51.100.0/24" {
		t.Fatalf("unexpected allowlist values: %+v", c.Auth.LoginIPAllowlist)
	}
}

func TestConfigValidateRejectsInvalidLoginIPAllowlist(t *testing.T) {
	c := &Config{
		App:      AppConfig{Env: "dev"},
		Server:   ServerConfig{Port: 8080},
		Database: DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Auth:     AuthConfig{JWTSecret: DefaultJWTSecretPlaceholder, LoginIPAllowlist: []string{"not-an-ip"}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected invalid login ip allowlist error")
	}
}
