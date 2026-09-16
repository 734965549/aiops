package config

import "testing"

func TestNormalizeCORSConfig_CredentialsNoWildcard(t *testing.T) {
	cfg := normalizeCORSConfig(CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	})
	if len(cfg.AllowOrigins) != 0 {
		t.Fatalf("expected wildcard stripped, got %+v", cfg.AllowOrigins)
	}
}

func TestNormalizeCORSConfig_CommaSeparatedOrigins(t *testing.T) {
	cfg := normalizeCORSConfig(CORSConfig{
		AllowOrigins:     []string{"http://localhost:5173,http://127.0.0.1:5173"},
		AllowCredentials: true,
	})
	if len(cfg.AllowOrigins) != 2 {
		t.Fatalf("expected 2 origins, got %+v", cfg.AllowOrigins)
	}
}

func TestNormalizeCORSConfig_EmptyOriginsWithCredentials(t *testing.T) {
	cfg := normalizeCORSConfig(CORSConfig{
		AllowOrigins:     nil,
		AllowCredentials: true,
	})
	if len(cfg.AllowOrigins) != 0 {
		t.Fatalf("expected empty origins after normalize, got %+v", cfg.AllowOrigins)
	}
}

func TestValidateCORS_ProdAllowsEmptyOriginsForSameOrigin(t *testing.T) {
	c := &Config{
		App:         AppConfig{Env: "prod"},
		Server:      ServerConfig{Port: 8080},
		Database:    DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Redis:       RedisConfig{Required: true},
		Auth:        AuthConfig{JWTSecret: "this-is-a-strong-enough-secret-for-prod-test-32b"},
		Integration: IntegrationConfig{CredentialEncryptionKey: strongCredentialEncryptionKey, CredentialEncryptionKeyVersion: 1},
		CORS:        CORSConfig{AllowCredentials: true},
		Execution:   ExecutionConfig{AgentRegisterToken: strongJWTSecret},
	}
	c.normalize()
	if err := c.Validate(); err != nil {
		t.Fatalf("expected prod cors validation to allow empty origins for same-origin reverse proxy, got %v", err)
	}
}

func TestValidateCORS_ProdRejectsWildcard(t *testing.T) {
	c := &Config{
		App:         AppConfig{Env: "prod"},
		Server:      ServerConfig{Port: 8080},
		Database:    DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Redis:       RedisConfig{Required: true},
		Auth:        AuthConfig{JWTSecret: "this-is-a-strong-enough-secret-for-prod-test-32b"},
		Integration: IntegrationConfig{CredentialEncryptionKey: strongCredentialEncryptionKey, CredentialEncryptionKeyVersion: 1},
		CORS:        CORSConfig{AllowOrigins: []string{"*"}},
	}
	c.normalize()
	if err := c.Validate(); err == nil {
		t.Fatal("expected prod cors validation error for wildcard origin")
	}
}

func TestNormalizeCORSConfig_DevFillsDefaultOrigin(t *testing.T) {
	c := &Config{
		App:  AppConfig{Env: "dev"},
		CORS: CORSConfig{AllowCredentials: true},
	}
	c.normalize()
	if len(c.CORS.AllowOrigins) != 1 || c.CORS.AllowOrigins[0] != defaultDevCORSOrigin {
		t.Fatalf("expected dev default origin, got %+v", c.CORS.AllowOrigins)
	}
}

func TestValidateCORS_RejectsWildcardWithCredentials(t *testing.T) {
	c := &Config{
		App:         AppConfig{Env: "dev"},
		Server:      ServerConfig{Port: 8080},
		Database:    DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Auth:        AuthConfig{JWTSecret: DefaultJWTSecretPlaceholder},
		Integration: devIntegrationConfig(),
		CORS:        CORSConfig{AllowOrigins: []string{"*"}, AllowCredentials: true},
	}
	c.normalize()
	if err := c.Validate(); err != nil {
		t.Fatalf("dev validate should pass after normalize strips wildcard, got %v", err)
	}
	if len(c.CORS.AllowOrigins) != 1 || c.CORS.AllowOrigins[0] != defaultDevCORSOrigin {
		t.Fatalf("expected dev default origin after wildcard stripped, got %+v", c.CORS.AllowOrigins)
	}
}
