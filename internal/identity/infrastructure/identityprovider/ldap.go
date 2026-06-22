package identityprovider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/go-ldap/ldap/v3"
)

const defaultLDAPTimeout = 10 * time.Second

// LDAPProvider 实现 LDAP / Active Directory 用户名密码认证。
type LDAPProvider struct {
	info   domain.ProviderInfo
	cfg    config.LDAPProviderConfig
	dialer ldapDialer
}

type ldapDialer interface {
	DialURL(addr string, opts ...ldap.DialOpt) (ldapConn, error)
}

type ldapConn interface {
	Bind(username, password string) error
	Search(searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error)
	StartTLS(config *tls.Config) error
	Close() error
}

type defaultLDAPDialer struct{}

func (defaultLDAPDialer) DialURL(addr string, opts ...ldap.DialOpt) (ldapConn, error) {
	return ldap.DialURL(addr, opts...)
}

// NewLDAPProvider 构造 LDAP/AD 身份源。
func NewLDAPProvider(info domain.ProviderInfo, cfg config.LDAPProviderConfig) (*LDAPProvider, error) {
	if strings.TrimSpace(info.ID) == "" {
		return nil, fmt.Errorf("ldap provider id is required")
	}
	if strings.TrimSpace(cfg.ServerURL) == "" {
		return nil, fmt.Errorf("ldap provider %q: server_url is required", info.ID)
	}
	if strings.TrimSpace(cfg.BaseDN) == "" {
		return nil, fmt.Errorf("ldap provider %q: base_dn is required", info.ID)
	}
	userFilter := strings.TrimSpace(cfg.UserFilter)
	if userFilter == "" {
		if info.Type == domain.ProviderTypeAD {
			userFilter = "(sAMAccountName={username})"
		} else {
			userFilter = "(uid={username})"
		}
	}
	cfg.UserFilter = userFilter
	if cfg.TimeoutS <= 0 {
		cfg.TimeoutS = 10
	}
	return &LDAPProvider{info: info, cfg: cfg, dialer: defaultLDAPDialer{}}, nil
}

// SetDialerForTest 允许跨包测试注入 mock LDAP 连接。
func (p *LDAPProvider) SetDialerForTest(d ldapDialer) {
	if p != nil && d != nil {
		p.dialer = d
	}
}

// Info 返回对外身份源摘要。
func (p *LDAPProvider) Info() domain.ProviderInfo {
	if p == nil {
		return domain.ProviderInfo{}
	}
	return p.info
}

// Authenticate 搜索用户 DN 并以用户凭据 bind 校验。
func (p *LDAPProvider) Authenticate(ctx context.Context, username, password string) (*domain.AuthenticatedExternalUser, error) {
	if p == nil {
		return nil, fmt.Errorf("ldap provider is nil")
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, domain.ErrInvalidCredentials
	}
	if len(username) > 64 || len(password) > 256 {
		return nil, domain.ErrInvalidCredentials
	}

	conn, err := p.connect()
	if err != nil {
		logger.From(ctx).Warn("ldap connect failed",
			logger.String("provider_id", p.info.ID),
			logger.Error(err),
		)
		return nil, domain.ErrInvalidCredentials
	}
	defer conn.Close()

	if bindDN := strings.TrimSpace(p.cfg.BindDN); bindDN != "" {
		if err := conn.Bind(bindDN, p.cfg.BindPassword); err != nil {
			logger.From(ctx).Warn("ldap service bind failed",
				logger.String("provider_id", p.info.ID),
				logger.Error(err),
			)
			return nil, domain.ErrInvalidCredentials
		}
	}

	filter := strings.ReplaceAll(p.cfg.UserFilter, "{username}", ldap.EscapeFilter(username))
	searchReq := ldap.NewSearchRequest(
		p.cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		filter,
		p.searchAttributes(),
		nil,
	)
	result, err := conn.Search(searchReq)
	if err != nil {
		logger.From(ctx).Warn("ldap user search failed",
			logger.String("provider_id", p.info.ID),
			logger.String("username", username),
			logger.Error(err),
		)
		return nil, domain.ErrInvalidCredentials
	}
	if len(result.Entries) == 0 {
		return nil, domain.ErrInvalidCredentials
	}
	if len(result.Entries) > 1 {
		logger.From(ctx).Warn("ldap user search ambiguous",
			logger.String("provider_id", p.info.ID),
			logger.String("username", username),
			logger.Int("count", len(result.Entries)),
		)
		return nil, domain.ErrInvalidCredentials
	}

	entry := result.Entries[0]
	userDN := entry.DN
	if err := conn.Bind(userDN, password); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	displayName := firstAttr(entry, p.cfg.AttrDisplayName, "cn", "displayName")
	email := firstAttr(entry, p.cfg.AttrEmail, "mail", "email")
	groups := collectGroups(entry, p.cfg.AttrGroups, "memberOf")

	return &domain.AuthenticatedExternalUser{
		ProviderID:      p.info.ID,
		ExternalSubject: p.entryExternalSubject(entry),
		Username:        username,
		DisplayName:     displayName,
		Email:           email,
		Groups:          groups,
	}, nil
}

func (p *LDAPProvider) connect() (ldapConn, error) {
	timeout := time.Duration(p.cfg.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = defaultLDAPTimeout
	}
	dialer := &net.Dialer{Timeout: timeout}
	opts := []ldap.DialOpt{ldap.DialWithDialer(dialer)}

	u, err := url.Parse(p.cfg.ServerURL)
	if err != nil {
		return nil, err
	}
	useTLS := strings.EqualFold(u.Scheme, "ldaps")
	if p.cfg.StartTLS && useTLS {
		return nil, fmt.Errorf("start_tls cannot be used with ldaps://")
	}

	tlsCfg, err := p.tlsConfig()
	if err != nil {
		return nil, err
	}
	if useTLS {
		opts = append(opts, ldap.DialWithTLSConfig(tlsCfg))
	}

	conn, err := p.dialer.DialURL(p.cfg.ServerURL, opts...)
	if err != nil {
		return nil, err
	}
	if p.cfg.StartTLS {
		if err := conn.StartTLS(tlsCfg); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

func (p *LDAPProvider) tlsConfig() (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if p.cfg.InsecureSkipVerify {
		cfg.InsecureSkipVerify = true
	}
	if caPath := strings.TrimSpace(p.cfg.CAFile); caPath != "" {
		data, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read ca file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(data) {
			return nil, fmt.Errorf("invalid ca file")
		}
		cfg.RootCAs = pool
	}
	return cfg, nil
}

func (p *LDAPProvider) searchAttributes() []string {
	attrs := []string{"dn"}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		for _, existing := range attrs {
			if strings.EqualFold(existing, name) {
				return
			}
		}
		attrs = append(attrs, name)
	}
	add(p.cfg.AttrDisplayName)
	add("cn")
	add("displayName")
	add(p.cfg.AttrEmail)
	add("mail")
	add(p.cfg.AttrGroups)
	add("memberOf")
	p.addSubjectAttributes(add)
	return attrs
}

func firstAttr(entry *ldap.Entry, preferred string, fallbacks ...string) string {
	if entry == nil {
		return ""
	}
	if v := strings.TrimSpace(entry.GetAttributeValue(preferred)); preferred != "" && v != "" {
		return v
	}
	for _, name := range fallbacks {
		if v := strings.TrimSpace(entry.GetAttributeValue(name)); v != "" {
			return v
		}
	}
	return ""
}

func collectGroups(entry *ldap.Entry, preferred string, fallbacks ...string) []string {
	if entry == nil {
		return nil
	}
	names := []string{preferred}
	names = append(names, fallbacks...)
	seen := make(map[string]struct{})
	var out []string
	for _, attr := range names {
		attr = strings.TrimSpace(attr)
		if attr == "" {
			continue
		}
		for _, v := range entry.GetAttributeValues(attr) {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			key := strings.ToLower(v)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// GroupRoleMapping 返回外部组到平台角色编码的映射。
func (p *LDAPProvider) GroupRoleMapping() map[string]string {
	if p == nil {
		return nil
	}
	return p.cfg.GroupRoleMapping
}

// AutoCreateUser 是否允许首次登录自动创建平台用户。
func (p *LDAPProvider) AutoCreateUser() bool {
	if p == nil {
		return false
	}
	return p.cfg.AutoCreateUser
}

// DefaultRoleCode 自动创建用户时的默认角色编码。
func (p *LDAPProvider) DefaultRoleCode() string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.cfg.DefaultRoleCode)
}
