package identityprovider

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/config"
)

// OAuth2Provider 实现 OAuth2 授权码流程（企业 SSO 可复用）。
type OAuth2Provider struct {
	info   domain.ProviderInfo
	cfg    config.OAuth2ProviderConfig
	client *http.Client
}

// NewOAuth2Provider 构造 OAuth2 身份源。
func NewOAuth2Provider(info domain.ProviderInfo, cfg config.OAuth2ProviderConfig) (*OAuth2Provider, error) {
	if strings.TrimSpace(info.ID) == "" {
		return nil, fmt.Errorf("oauth2 provider id is required")
	}
	if strings.TrimSpace(cfg.AuthorizationURL) == "" || strings.TrimSpace(cfg.TokenURL) == "" {
		return nil, fmt.Errorf("oauth2 provider %q: authorization_url and token_url are required", info.ID)
	}
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.RedirectURI) == "" {
		return nil, fmt.Errorf("oauth2 provider %q: client_id and redirect_uri are required", info.ID)
	}
	timeout := time.Duration(cfg.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &OAuth2Provider{
		info:   info,
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}, nil
}

func (p *OAuth2Provider) Info() domain.ProviderInfo {
	if p == nil {
		return domain.ProviderInfo{}
	}
	return p.info
}

func (p *OAuth2Provider) AuthorizationURL(state string) (string, error) {
	if p == nil {
		return "", fmt.Errorf("oauth2 provider is nil")
	}
	u, err := url.Parse(p.cfg.AuthorizationURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", p.cfg.ClientID)
	q.Set("redirect_uri", p.cfg.RedirectURI)
	q.Set("state", state)
	if len(p.cfg.Scopes) > 0 {
		q.Set("scope", strings.Join(p.cfg.Scopes, " "))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (p *OAuth2Provider) ExchangeCode(ctx context.Context, code string) (*domain.AuthenticatedExternalUser, error) {
	if p == nil {
		return nil, fmt.Errorf("oauth2 provider is nil")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, domain.ErrInvalidCredentials
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", p.cfg.RedirectURI)
	form.Set("client_id", p.cfg.ClientID)
	if p.cfg.ClientSecret != "" {
		form.Set("client_secret", p.cfg.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domain.ErrInvalidCredentials
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil || tokenResp.AccessToken == "" {
		return nil, domain.ErrInvalidCredentials
	}
	return p.fetchUserInfo(ctx, tokenResp.AccessToken)
}

func (p *OAuth2Provider) fetchUserInfo(ctx context.Context, accessToken string) (*domain.AuthenticatedExternalUser, error) {
	if strings.TrimSpace(p.cfg.UserInfoURL) == "" {
		return nil, fmt.Errorf("oauth2 provider %q: userinfo_url is required", p.info.ID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domain.ErrInvalidCredentials
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	subject := pickString(raw, p.cfg.SubjectClaim, "sub", "id", "user_id")
	username := pickString(raw, p.cfg.UsernameClaim, "preferred_username", "username", "login", "name")
	displayName := pickString(raw, p.cfg.DisplayNameClaim, "name", "display_name", "nickname")
	email := pickString(raw, p.cfg.EmailClaim, "email")
	groups := pickGroups(raw, p.cfg.GroupsClaim, "groups", "roles", "group")
	if subject == "" {
		subject = username
	}
	if subject == "" {
		return nil, domain.ErrInvalidCredentials
	}
	if username == "" {
		username = subject
	}
	return &domain.AuthenticatedExternalUser{
		ProviderID:      p.info.ID,
		ExternalSubject: subject,
		Username:        username,
		DisplayName:     displayName,
		Email:           email,
		Groups:          groups,
	}, nil
}

func pickString(raw map[string]any, preferred string, fallbacks ...string) string {
	if v := lookupString(raw, preferred); preferred != "" && v != "" {
		return v
	}
	for _, key := range fallbacks {
		if v := lookupString(raw, key); v != "" {
			return v
		}
	}
	return ""
}

func lookupString(raw map[string]any, key string) string {
	key = strings.TrimSpace(key)
	if key == "" || raw == nil {
		return ""
	}
	v, ok := raw[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func pickGroups(raw map[string]any, preferred string, fallbacks ...string) []string {
	keys := make([]string, 0, 1+len(fallbacks))
	if preferred = strings.TrimSpace(preferred); preferred != "" {
		keys = append(keys, preferred)
	}
	keys = append(keys, fallbacks...)
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || raw == nil {
			continue
		}
		if groups := parseGroupsValue(raw[key]); len(groups) > 0 {
			return groups
		}
	}
	return nil
}

func parseGroupsValue(v any) []string {
	switch val := v.(type) {
	case string:
		return splitCommaGroups(val)
	case []string:
		return normalizeGroups(val)
	case []any:
		items := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				continue
			}
			items = append(items, s)
		}
		return normalizeGroups(items)
	default:
		return nil
	}
}

func splitCommaGroups(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return normalizeGroups(strings.Split(s, ","))
}

func normalizeGroups(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// OAuth2ProviderExtras 暴露 OAuth2 Provisioning 配置。
type OAuth2ProviderExtras struct {
	AutoCreateUser  bool
	DefaultRoleCode string
	GroupRoleMap    map[string]string
}

// Extras 返回账号同步相关配置。
func (p *OAuth2Provider) Extras() OAuth2ProviderExtras {
	if p == nil {
		return OAuth2ProviderExtras{}
	}
	return OAuth2ProviderExtras{
		AutoCreateUser:  p.cfg.AutoCreateUser,
		DefaultRoleCode: strings.TrimSpace(p.cfg.DefaultRoleCode),
		GroupRoleMap:    p.cfg.GroupRoleMapping,
	}
}

// NewOAuthState 生成 OAuth state 参数。
func NewOAuthState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
