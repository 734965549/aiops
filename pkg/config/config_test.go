package config

import "testing"

func TestLoadReadsRedisPasswordFromEnv(t *testing.T) {
	t.Setenv("AIOPS_REDIS__ADDR", "10.51.2.220:6379")
	t.Setenv("AIOPS_REDIS__USERNAME", "default")
	t.Setenv("AIOPS_REDIS__PASSWORD", "ubTKSLICMXs=")
	t.Setenv("AIOPS_REDIS__DB", "2")
	t.Setenv("AIOPS_REDIS__REQUIRED", "true")

	c, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if c.Redis.Addr != "10.51.2.220:6379" {
		t.Fatalf("unexpected redis addr: %q", c.Redis.Addr)
	}
	if c.Redis.Username != "default" {
		t.Fatalf("unexpected redis username: %q", c.Redis.Username)
	}
	if c.Redis.Password != "ubTKSLICMXs=" {
		t.Fatalf("unexpected redis password: %q", c.Redis.Password)
	}
	if c.Redis.DB != 2 {
		t.Fatalf("unexpected redis db: %d", c.Redis.DB)
	}
	if !c.Redis.Required {
		t.Fatal("expected redis.required from env")
	}
}

func TestConfigValidateAIExample(t *testing.T) {
	c := &Config{
		App:          AppConfig{Env: "dev"},
		Server:       ServerConfig{Port: 8080},
		Database:     DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Auth:         AuthConfig{JWTSecret: DefaultJWTSecretPlaceholder},
		Integration:  devIntegrationConfig(),
		AI:           AIConfig{Providers: []AIProviderConfig{{ID: "demo-http-a", Name: "Demo HTTP Provider A", Type: "a", BaseURL: "http://127.0.0.1:9000", APIKey: "demo-api-key", TimeoutMS: 30000, Enabled: true}}},
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
		App:         AppConfig{Env: "prod"},
		Server:      ServerConfig{Port: 8080},
		Database:    DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Redis:       RedisConfig{Required: false},
		Auth:        AuthConfig{JWTSecret: "this-is-a-strong-enough-secret-for-prod-test-32b"},
		Integration: IntegrationConfig{CredentialEncryptionKey: strongCredentialEncryptionKey, CredentialEncryptionKeyVersion: 1},
		CORS:        CORSConfig{AllowOrigins: []string{"https://app.example.com"}},
	}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected validate error when redis.required is false in prod")
	}
	c.Redis.Required = true
	if err := c.Validate(); err != nil {
		t.Fatalf("expected validate success with redis.required in prod, got %v", err)
	}
}

func TestConfigValidateRejectsInvalidLoggerConfig(t *testing.T) {
	c := &Config{
		App:         AppConfig{Env: "dev"},
		Server:      ServerConfig{Port: 8080},
		Logger:      LoggerConfig{Level: "fatal", Format: "json"},
		Database:    DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Auth:        AuthConfig{JWTSecret: DefaultJWTSecretPlaceholder},
		Integration: devIntegrationConfig(),
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected invalid logger level error")
	}

	c.Logger.Level = "info"
	c.Logger.Format = "xml"
	if err := c.Validate(); err == nil {
		t.Fatal("expected invalid logger format error")
	}

	c.Logger.Format = "json"
	c.Logger.Output = "stdot"
	if err := c.Validate(); err == nil {
		t.Fatal("expected invalid logger output error")
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
		App:         AppConfig{Env: "dev"},
		Server:      ServerConfig{Port: 8080},
		Database:    DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Auth:        AuthConfig{JWTSecret: DefaultJWTSecretPlaceholder, LoginIPAllowlist: []string{"not-an-ip"}},
		Integration: devIntegrationConfig(),
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected invalid login ip allowlist error")
	}
}
