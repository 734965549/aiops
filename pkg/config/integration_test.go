package config

import "testing"

const strongCredentialEncryptionKey = "yM8#nP3@wL6oQ9xR4!zT8%vU5^cO7&dH2*fI1-gK3-jL4"

func devIntegrationConfig() IntegrationConfig {
	return IntegrationConfig{
		CredentialEncryptionKey:        DefaultCredentialEncryptionKeyPlaceholder,
		CredentialEncryptionKeyVersion: 1,
	}
}

func TestValidateCredentialEncryptionKeyDevAllowsPlaceholder(t *testing.T) {
	if err := ValidateCredentialEncryptionKey(DefaultCredentialEncryptionKeyPlaceholder, "dev"); err != nil {
		t.Fatalf("dev should allow placeholder, got %v", err)
	}
}

func TestValidateCredentialEncryptionKeyRejectsWeakNonDev(t *testing.T) {
	if err := ValidateCredentialEncryptionKey(DefaultCredentialEncryptionKeyPlaceholder, "prod"); err == nil {
		t.Fatal("expected rejection for dev placeholder in prod")
	}
}

func TestValidateIntegrationConfigRejectsSameKeyAsJWTNonDev(t *testing.T) {
	secret := strongJWTSecret
	cfg := IntegrationConfig{
		CredentialEncryptionKey:        secret,
		CredentialEncryptionKeyVersion: 1,
	}
	if err := validateIntegrationConfig(cfg, "prod", secret); err == nil {
		t.Fatal("expected rejection when credential key equals jwt secret")
	}
}

func TestConfigValidateRequiresIntegrationKey(t *testing.T) {
	c := &Config{
		App:      AppConfig{Env: "dev"},
		Server:   ServerConfig{Port: 8080},
		Database: DatabaseConfig{Host: "127.0.0.1", Name: "aiops"},
		Auth:     AuthConfig{JWTSecret: DefaultJWTSecretPlaceholder},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected validate error when integration key missing")
	}
	c.Integration = devIntegrationConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("expected validate success, got %v", err)
	}
}
