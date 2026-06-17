package config

import (
	"fmt"
	"math"
	"strings"
	"unicode"
)

const (
	// DefaultJWTSecretPlaceholder 是 config.example.yaml 中的占位值，仅允许 dev 环境使用。
	DefaultJWTSecretPlaceholder = "please-change-me-in-production"

	// DevJWTSecretPlaceholder 是本地 / Compose dev 联调专用占位值，禁止用于非 dev 环境。
	DevJWTSecretPlaceholder = "dev-only-not-secret-please-change"

	minJWTSecretLenNonDev = 32
	minJWTSecretUnique    = 16
	minJWTSecretEntropy   = 3.8
	maxJWTSecretRepeatRun = 4
)

// weakJWTSecrets 收录常见弱密钥与项目内置占位值（比较时不区分大小写）。
var weakJWTSecrets = []string{
	DefaultJWTSecretPlaceholder,
	DevJWTSecretPlaceholder,
	"secret",
	"changeme",
	"change-me",
	"password",
	"passw0rd",
	"jwt-secret",
	"jwt_secret",
	"jwtsecret",
	"your-secret-key",
	"your-256-bit-secret",
	"supersecret",
	"super-secret",
	"mysecret",
	"my-secret",
	"test-secret",
	"testsecret",
	"keyboardcat",
	"12345678901234567890123456789012",
	"abcdefghijklmnopqrstuvwxyzabcdef",
}

// ValidateJWTSecret 按运行环境校验 JWT 对称密钥强度。
//
// dev：允许内置占位值或任意自定义值（便于本地联调）。
// 非 dev：拒绝占位值、弱密钥列表项，并要求长度、字符多样性、熵与重复模式达标。
func ValidateJWTSecret(secret, env string) error {
	if strings.TrimSpace(secret) == "" {
		return fmt.Errorf("auth.jwt_secret must not be empty")
	}
	if env == "dev" {
		return nil
	}
	if isWeakJWTSecret(secret) {
		return fmt.Errorf("auth.jwt_secret is a known weak or placeholder value (env=%q)", env)
	}
	if len(secret) < minJWTSecretLenNonDev {
		return fmt.Errorf("auth.jwt_secret too short (>=32 bytes required for env=%q)", env)
	}
	if uniqueRunes(secret) < minJWTSecretUnique {
		return fmt.Errorf("auth.jwt_secret lacks character diversity (env=%q)", env)
	}
	if maxRepeatRun(secret) > maxJWTSecretRepeatRun {
		return fmt.Errorf("auth.jwt_secret contains excessive repeated characters (env=%q)", env)
	}
	entropy := shannonEntropy(secret)
	classes := countCharClasses(secret)
	if classes < 3 && entropy < minJWTSecretEntropy+0.7 {
		return fmt.Errorf("auth.jwt_secret must include more character classes or higher entropy (env=%q)", env)
	}
	if entropy < minJWTSecretEntropy {
		return fmt.Errorf("auth.jwt_secret entropy too low (env=%q)", env)
	}
	return nil
}

func isWeakJWTSecret(secret string) bool {
	normalized := strings.ToLower(strings.TrimSpace(secret))
	for _, weak := range weakJWTSecrets {
		if normalized == strings.ToLower(weak) {
			return true
		}
	}
	return false
}

func uniqueRunes(s string) int {
	seen := make(map[rune]struct{}, len(s))
	for _, r := range s {
		seen[r] = struct{}{}
	}
	return len(seen)
}

func maxRepeatRun(s string) int {
	if len(s) == 0 {
		return 0
	}
	maxRun := 1
	run := 1
	prev := rune(s[0])
	for _, r := range s[1:] {
		if r == prev {
			run++
			if run > maxRun {
				maxRun = run
			}
			continue
		}
		prev = r
		run = 1
	}
	return maxRun
}

func countCharClasses(s string) int {
	var hasUpper, hasLower, hasDigit, hasOther bool
	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasOther = true
		}
	}
	classes := 0
	if hasUpper {
		classes++
	}
	if hasLower {
		classes++
	}
	if hasDigit {
		classes++
	}
	if hasOther {
		classes++
	}
	return classes
}

func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]int, len(s))
	for _, r := range s {
		freq[r]++
	}
	var entropy float64
	n := float64(len(s))
	for _, count := range freq {
		p := float64(count) / n
		entropy -= p * math.Log2(p)
	}
	return entropy
}
