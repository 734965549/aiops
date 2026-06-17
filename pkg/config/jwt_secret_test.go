package config

import "testing"

// strongJWTSecret 满足非 dev 环境的熵、长度与字符多样性要求。
const strongJWTSecret = "xK9#mP2@vL5nQ8wR3!yT7%zU4^bN6&cF1*dG0-hJ2"

func TestValidateJWTSecretDevAllowsPlaceholders(t *testing.T) {
	for _, secret := range []string{DefaultJWTSecretPlaceholder, DevJWTSecretPlaceholder, "weak"} {
		if err := ValidateJWTSecret(secret, "dev"); err != nil {
			t.Fatalf("dev should allow %q, got %v", secret, err)
		}
	}
}

func TestValidateJWTSecretRejectsWeakNonDev(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		env    string
	}{
		{name: "yaml placeholder", secret: DefaultJWTSecretPlaceholder, env: "prod"},
		{name: "compose dev placeholder", secret: DevJWTSecretPlaceholder, env: "staging"},
		{name: "known weak secret", secret: "super-secret-key-for-jwt-signing-now", env: "prod"},
		{name: "low entropy phrase", secret: "please-change-me-in-production-ok", env: "test"},
		{name: "repeated chars", secret: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", env: "prod"},
		{name: "too short", secret: "short-secret", env: "prod"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateJWTSecret(tc.secret, tc.env); err == nil {
				t.Fatalf("expected rejection for %q in env=%q", tc.secret, tc.env)
			}
		})
	}
}

func TestValidateJWTSecretAcceptsStrongNonDev(t *testing.T) {
	for _, env := range []string{"test", "staging", "prod"} {
		if err := ValidateJWTSecret(strongJWTSecret, env); err != nil {
			t.Fatalf("expected strong secret accepted in env=%q, got %v", env, err)
		}
	}
}

func TestConfigValidateRejectsComposeDevSecretInProd(t *testing.T) {
	c := &Config{
		App:      AppConfig{Env: "prod"},
		Server:   ServerConfig{Port: 8080},
		Database: DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Auth:     AuthConfig{JWTSecret: DevJWTSecretPlaceholder},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected validate error for prod dev-only jwt secret")
	}
}
