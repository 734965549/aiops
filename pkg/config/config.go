// Package config 负责从 YAML 文件与环境变量加载平台配置。
//
// 加载顺序（高优先级覆盖低优先级）：
//  1. 环境变量（前缀 AIOPS_，分段分隔符为双下划线 "__"，
//     例：AIOPS_DATABASE__PASSWORD=xxx 对应 database.password）。
//  2. configs/config.yaml 或 -config 指定的文件（如果存在）。
//  3. 默认值（通过 setDefaults 注册）。
//
// 业务模块通过传入 *Config 字段（而不是包级变量）使用配置，避免隐式全局耦合，
// 也方便在单元测试中以结构体字面量构造定制配置。
package config

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 是平台运行所需的全部配置项的聚合根。
type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Server   ServerConfig   `mapstructure:"server"`
	Logger   LoggerConfig   `mapstructure:"logger"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Identity IdentityConfig `mapstructure:"identity"`
	CORS     CORSConfig     `mapstructure:"cors"`
	AI       AIConfig       `mapstructure:"ai"`
}

// AppConfig 描述应用元数据。
type AppConfig struct {
	Name              string `mapstructure:"name"`
	Env               string `mapstructure:"env"` // dev / test / prod
	Timezone          string `mapstructure:"timezone"`
	BootstrapTimeoutS int    `mapstructure:"bootstrap_timeout_s"`
}

// ServerConfig 描述 HTTP 服务监听参数。
type ServerConfig struct {
	Host             string `mapstructure:"host"`
	Port             int    `mapstructure:"port"`
	ReadTimeoutS     int    `mapstructure:"read_timeout_s"`
	WriteTimeoutS    int    `mapstructure:"write_timeout_s"`
	ShutdownTimeoutS int    `mapstructure:"shutdown_timeout_s"`
}

// LoggerConfig 描述日志参数。
type LoggerConfig struct {
	Level      string `mapstructure:"level"`  // debug / info / warn / error
	Format     string `mapstructure:"format"` // json / console
	Output     string `mapstructure:"output"` // stdout / stderr / file
	FilePath   string `mapstructure:"file_path"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
}

// DatabaseConfig 描述 PostgreSQL 连接参数。
type DatabaseConfig struct {
	Driver           string `mapstructure:"driver"`
	Host             string `mapstructure:"host"`
	Port             int    `mapstructure:"port"`
	User             string `mapstructure:"user"`
	Password         string `mapstructure:"password"`
	Name             string `mapstructure:"name"`
	SSLMode          string `mapstructure:"ssl_mode"`
	MaxIdleConns     int    `mapstructure:"max_idle_conns"`
	MaxOpenConns     int    `mapstructure:"max_open_conns"`
	ConnMaxLifetimeS int    `mapstructure:"conn_max_lifetime_s"`
	LogLevel         string `mapstructure:"log_level"` // silent / error / warn / info
	AutoMigrate      bool   `mapstructure:"auto_migrate"`
	MigrateTimeoutS  int    `mapstructure:"migrate_timeout_s"`
}

// DSN 拼装 PostgreSQL 连接串。
func (d DatabaseConfig) DSN(tz string) string {
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode, tz,
	)
}

// ConnMaxLifetime 把秒级配置转换为 time.Duration，便于 GORM/database 使用。
func (d DatabaseConfig) ConnMaxLifetime() time.Duration {
	if d.ConnMaxLifetimeS <= 0 {
		return time.Hour
	}
	return time.Duration(d.ConnMaxLifetimeS) * time.Second
}

// RedisConfig 描述 Redis 连接参数。
type RedisConfig struct {
	Required bool   `mapstructure:"required"`
	Addr     string `mapstructure:"addr"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

// AuthConfig 描述鉴权相关配置。
//
// 注意：JWTSecret 在非 dev 环境必须通过环境变量注入强密钥（AIOPS_AUTH__JWT_SECRET）。
// dev 可使用 DefaultJWTSecretPlaceholder 或 DevJWTSecretPlaceholder；非 dev 会拒绝占位值、
// 弱密钥列表项，并校验长度、字符多样性与熵。
type AuthConfig struct {
	JWTSecret            string `mapstructure:"jwt_secret"`
	JWTIssuer            string `mapstructure:"jwt_issuer"`
	AccessTTLM           int    `mapstructure:"access_ttl_m"`
	RefreshTTLH          int    `mapstructure:"refresh_ttl_h"`
	BootstrapUsername    string `mapstructure:"bootstrap_username"`
	BootstrapPassword    string `mapstructure:"bootstrap_password"`
	BootstrapDisplayName string `mapstructure:"bootstrap_display_name"`
	// GrantCacheTTLS 控制 LoadUserGrantContext 的 Redis 短期缓存秒数；0 表示禁用。
	GrantCacheTTLS int                  `mapstructure:"grant_cache_ttl_s"`
	LoginRateLimit LoginRateLimitConfig `mapstructure:"login_rate_limit"`
	// LoginIPAllowlist 控制登录相关入口嘅来源 IP；空列表表示唔限制。
	LoginIPAllowlist []string `mapstructure:"login_ip_allowlist"`
}

// LoginRateLimitConfig 控制登录/刷新接口的 IP 与用户名维度限流。
type LoginRateLimitConfig struct {
	Enabled                       bool `mapstructure:"enabled"`
	IPRequestsPerWindow           int  `mapstructure:"ip_requests_per_window"`
	IPWindowS                     int  `mapstructure:"ip_window_s"`
	IPFailuresBeforeLockout       int  `mapstructure:"ip_failures_before_lockout"`
	UsernameFailuresBeforeLockout int  `mapstructure:"username_failures_before_lockout"`
	LockoutS                      int  `mapstructure:"lockout_s"`
}

// AccessTTL 返回 access token 过期时长。
func (a AuthConfig) AccessTTL() time.Duration {
	if a.AccessTTLM <= 0 {
		return 2 * time.Hour
	}
	return time.Duration(a.AccessTTLM) * time.Minute
}

// RefreshTTL 返回 refresh token 过期时长。
func (a AuthConfig) RefreshTTL() time.Duration {
	if a.RefreshTTLH <= 0 {
		return 7 * 24 * time.Hour
	}
	return time.Duration(a.RefreshTTLH) * time.Hour
}

// GrantCacheTTL 返回用户授权聚合缓存 TTL；Redis 不可用或配置为 0 时禁用。
func (a AuthConfig) GrantCacheTTL() time.Duration {
	if a.GrantCacheTTLS <= 0 {
		return 0
	}
	return time.Duration(a.GrantCacheTTLS) * time.Second
}

// IdentityConfig 描述企业身份源配置。
type IdentityConfig struct {
	Providers []IdentityProviderConfig `mapstructure:"providers"`
}

// IdentityProviderConfig 描述单个 LDAP / AD / OAuth2 / OIDC / SSO 身份源。
type IdentityProviderConfig struct {
	ID       string               `mapstructure:"id"`
	Type     string               `mapstructure:"type"` // ldap | ad | oauth2 | oidc | sso
	Name     string               `mapstructure:"name"`
	Enabled  bool                 `mapstructure:"enabled"`
	Priority int                  `mapstructure:"priority"`
	LDAP     LDAPProviderConfig   `mapstructure:"ldap"`
	OAuth2   OAuth2ProviderConfig `mapstructure:"oauth2"`
	OIDC     OIDCProviderConfig   `mapstructure:"oidc"`
}

// LDAPProviderConfig LDAP / Active Directory 连接与同步参数。
type LDAPProviderConfig struct {
	ServerURL          string            `mapstructure:"server_url"`
	BindDN             string            `mapstructure:"bind_dn"`
	BindPassword       string            `mapstructure:"bind_password"`
	BaseDN             string            `mapstructure:"base_dn"`
	UserFilter         string            `mapstructure:"user_filter"`
	StartTLS           bool              `mapstructure:"start_tls"`
	InsecureSkipVerify bool              `mapstructure:"insecure_skip_verify"`
	CAFile             string            `mapstructure:"ca_file"`
	TimeoutS           int               `mapstructure:"timeout_s"`
	AttrDisplayName    string            `mapstructure:"attr_display_name"`
	AttrEmail          string            `mapstructure:"attr_email"`
	AttrGroups         string            `mapstructure:"attr_groups"`
	AttrSubject        string            `mapstructure:"attr_subject"`
	BrowseOrgFilter    string            `mapstructure:"browse_org_filter"`
	BrowseUserFilter   string            `mapstructure:"browse_user_filter"`
	AutoCreateUser     bool              `mapstructure:"auto_create_user"`
	DefaultRoleCode    string            `mapstructure:"default_role_code"`
	GroupRoleMapping   map[string]string `mapstructure:"group_role_mapping"`
}

// OAuth2ProviderConfig OAuth2 / 企业 SSO 参数。
type OAuth2ProviderConfig struct {
	AuthorizationURL string            `mapstructure:"authorization_url"`
	TokenURL         string            `mapstructure:"token_url"`
	UserInfoURL      string            `mapstructure:"userinfo_url"`
	ClientID         string            `mapstructure:"client_id"`
	ClientSecret     string            `mapstructure:"client_secret"`
	RedirectURI      string            `mapstructure:"redirect_uri"`
	Scopes           []string          `mapstructure:"scopes"`
	TimeoutS         int               `mapstructure:"timeout_s"`
	SubjectClaim     string            `mapstructure:"subject_claim"`
	UsernameClaim    string            `mapstructure:"username_claim"`
	DisplayNameClaim string            `mapstructure:"display_name_claim"`
	EmailClaim       string            `mapstructure:"email_claim"`
	GroupsClaim      string            `mapstructure:"groups_claim"`
	AutoCreateUser   bool              `mapstructure:"auto_create_user"`
	DefaultRoleCode  string            `mapstructure:"default_role_code"`
	GroupRoleMapping map[string]string `mapstructure:"group_role_mapping"`
}

// OIDCProviderConfig OpenID Connect 参数。
type OIDCProviderConfig struct {
	Issuer           string            `mapstructure:"issuer"`
	ClientID         string            `mapstructure:"client_id"`
	ClientSecret     string            `mapstructure:"client_secret"`
	RedirectURI      string            `mapstructure:"redirect_uri"`
	Scopes           []string          `mapstructure:"scopes"`
	TimeoutS         int               `mapstructure:"timeout_s"`
	UsernameClaim    string            `mapstructure:"username_claim"`
	DisplayNameClaim string            `mapstructure:"display_name_claim"`
	EmailClaim       string            `mapstructure:"email_claim"`
	GroupsClaim      string            `mapstructure:"groups_claim"`
	AutoCreateUser   bool              `mapstructure:"auto_create_user"`
	DefaultRoleCode  string            `mapstructure:"default_role_code"`
	GroupRoleMapping map[string]string `mapstructure:"group_role_mapping"`
}

// CORSConfig 描述跨域策略。
type CORSConfig struct {
	AllowOrigins     []string `mapstructure:"allow_origins"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
}

// AIConfig 描述 AI 工具 provider 配置。
//
// 配置示例：
//
//	ai:
//	  providers:
//	    - id: demo-http-a
//	      name: Demo HTTP Provider A
//	      type: a
//	      base_url: http://127.0.0.1:9000
//	      api_key: <REPLACE_WITH_YOUR_API_KEY>  # 仅占位，禁止用于真实环境
//	      timeout_ms: 30000
//	      headers:
//	        X-Client-Name: aiops
//	      enabled: true
//	      description: 通用 HTTP API Key 工具提供方样例
//
// 环境变量示例：
//
//	AIOPS_AI__PROVIDERS 目前不做整段 JSON 反序列化，建议优先通过 YAML 维护 provider 列表。
//
// 启动时 cmd/api 会将 ai.providers 载入 toolgateway.ProviderRegistry 内存注册表；
// 纯 env 启动时可改用 POST /api/ai/providers API 维护。
type AIConfig struct {
	Providers             []AIProviderConfig `mapstructure:"providers"`
	OutboundAllowedHosts  []string           `mapstructure:"outbound_allowed_hosts"`
	OutboundAllowLoopback bool               `mapstructure:"outbound_allow_loopback"`
}

// AIProviderConfig 描述单个 provider 的配置。
type AIProviderConfig struct {
	ID          string            `mapstructure:"id"`
	Name        string            `mapstructure:"name"`
	Type        string            `mapstructure:"type"`
	BaseURL     string            `mapstructure:"base_url"`
	APIKey      string            `mapstructure:"api_key"`
	TimeoutMS   int64             `mapstructure:"timeout_ms"`
	Headers     map[string]string `mapstructure:"headers"`
	Enabled     bool              `mapstructure:"enabled"`
	Description string            `mapstructure:"description"`
}

// Load 读取配置文件并合并环境变量。configPath 为空时使用默认路径。
func Load(configPath string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetEnvPrefix("AIOPS")
	v.AutomaticEnv()
	// yaml 用 "." 表示分段，环境变量分段使用双下划线 "__"，
	// 避免与 snake_case 键名内部的单下划线发生歧义（如 database.ssl_mode）。
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		// 配置文件不是必需的：允许仅依靠环境变量与默认值启动（例如容器内）。
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	c.normalize()
	return &c, nil
}

// normalize 修正环境变量与 viper 合并后的边界情况（如 slice 逗号分隔、CORS 安全回退）。
func (c *Config) normalize() {
	c.CORS = normalizeCORSConfig(c.CORS)
	c.Auth.LoginIPAllowlist = normalizeStringList(c.Auth.LoginIPAllowlist)
}

const defaultDevCORSOrigin = "http://localhost:5173"

// DefaultDevCORSOrigin 返回 dev 环境 CORS 默认 origin。
func DefaultDevCORSOrigin() string { return defaultDevCORSOrigin }

// normalizeCORSConfig 展开逗号分隔的 origin，并在 allow_credentials=true 时禁止 * 回退。
func normalizeCORSConfig(cfg CORSConfig) CORSConfig {
	expanded := normalizeStringList(cfg.AllowOrigins)

	if cfg.AllowCredentials {
		filtered := make([]string, 0, len(expanded))
		for _, origin := range expanded {
			if origin != "*" {
				filtered = append(filtered, origin)
			}
		}
		expanded = filtered
	}

	return CORSConfig{
		AllowOrigins:     expanded,
		AllowCredentials: cfg.AllowCredentials,
	}
}

// normalizeStringList 展开逗号分隔嘅配置项，方便 YAML 同环境变量共用同一套解析。
func normalizeStringList(items []string) []string {
	expanded := make([]string, 0, len(items))
	for _, item := range items {
		for _, part := range strings.Split(item, ",") {
			if s := strings.TrimSpace(part); s != "" {
				expanded = append(expanded, s)
			}
		}
	}
	return expanded
}

// Validate 在初始化基础组件前做一次配置健全性检查。
// 仅校验「错了就会让平台跑不起来或留下安全隐患」的项。
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port %d invalid", c.Server.Port)
	}
	if c.Database.Host == "" || c.Database.Name == "" {
		return fmt.Errorf("database.host / database.name must not be empty")
	}
	if err := ValidateJWTSecret(c.Auth.JWTSecret, c.App.Env); err != nil {
		return err
	}
	if c.Auth.BootstrapUsername != "" || c.Auth.BootstrapPassword != "" {
		if c.App.Env == "prod" {
			return fmt.Errorf("auth bootstrap user is not allowed in prod")
		}
		if strings.TrimSpace(c.Auth.BootstrapUsername) == "" || len(c.Auth.BootstrapPassword) < 8 {
			return fmt.Errorf("auth.bootstrap_username is required and auth.bootstrap_password must be >=8 chars when bootstrap is enabled")
		}
	}
	if err := validateCORS(c.App.Env, c.CORS); err != nil {
		return err
	}
	if err := validateIPAllowlist(c.Auth.LoginIPAllowlist); err != nil {
		return err
	}
	if c.App.Env == "prod" && !c.Redis.Required {
		return fmt.Errorf("redis.required must be true in prod (refresh token session store)")
	}
	return nil
}

func validateCORS(env string, cfg CORSConfig) error {
	if cfg.AllowCredentials {
		for _, origin := range cfg.AllowOrigins {
			if origin == "*" {
				return fmt.Errorf("cors.allow_origins must not contain * when cors.allow_credentials is true")
			}
		}
	}
	if env == "prod" {
		if len(cfg.AllowOrigins) == 0 {
			return fmt.Errorf("cors.allow_origins must be explicitly configured in prod")
		}
		for _, origin := range cfg.AllowOrigins {
			if origin == "*" {
				return fmt.Errorf("cors.allow_origins must not contain * in prod")
			}
		}
	}
	return nil
}

// validateIPAllowlist 校验登录 IP 白名单入面嘅单 IP 同 CIDR 网段。
func validateIPAllowlist(entries []string) error {
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return fmt.Errorf("auth.login_ip_allowlist contains invalid cidr %q", entry)
			}
			continue
		}
		if net.ParseIP(entry) == nil {
			return fmt.Errorf("auth.login_ip_allowlist contains invalid ip %q", entry)
		}
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "aiops")
	v.SetDefault("app.env", "dev")
	v.SetDefault("app.timezone", "Asia/Shanghai")
	v.SetDefault("app.bootstrap_timeout_s", 30)

	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout_s", 15)
	v.SetDefault("server.write_timeout_s", 30)
	v.SetDefault("server.shutdown_timeout_s", 10)

	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.format", "console")
	v.SetDefault("logger.output", "stdout")
	v.SetDefault("logger.file_path", "logs/aiops.log")
	v.SetDefault("logger.max_size_mb", 100)
	v.SetDefault("logger.max_backups", 10)
	v.SetDefault("logger.max_age_days", 30)

	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.host", "127.0.0.1")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "aiops")
	v.SetDefault("database.password", "aiops")
	v.SetDefault("database.name", "aiops")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime_s", 60)
	v.SetDefault("database.log_level", "warn")
	v.SetDefault("database.auto_migrate", false)
	v.SetDefault("database.migrate_timeout_s", 300)

	v.SetDefault("redis.required", false)
	v.SetDefault("redis.addr", "127.0.0.1:6379")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.pool_size", 50)

	v.SetDefault("auth.jwt_secret", DefaultJWTSecretPlaceholder)
	v.SetDefault("auth.jwt_issuer", "aiops")
	v.SetDefault("auth.access_ttl_m", 120)
	v.SetDefault("auth.refresh_ttl_h", 168)
	v.SetDefault("auth.bootstrap_username", "")
	v.SetDefault("auth.bootstrap_password", "")
	v.SetDefault("auth.bootstrap_display_name", "Administrator")
	v.SetDefault("auth.grant_cache_ttl_s", 60)
	v.SetDefault("auth.login_ip_allowlist", []string{})
	v.SetDefault("auth.login_rate_limit.enabled", true)
	v.SetDefault("auth.login_rate_limit.ip_requests_per_window", 30)
	v.SetDefault("auth.login_rate_limit.ip_window_s", 60)
	v.SetDefault("auth.login_rate_limit.ip_failures_before_lockout", 20)
	v.SetDefault("auth.login_rate_limit.username_failures_before_lockout", 5)
	v.SetDefault("auth.login_rate_limit.lockout_s", 900)

	v.SetDefault("cors.allow_origins", []string{"http://localhost:5173"})
	v.SetDefault("cors.allow_credentials", true)
}
