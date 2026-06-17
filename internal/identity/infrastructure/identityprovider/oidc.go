package identityprovider

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/config"
)

// OIDCProvider 基于 OpenID Connect Discovery 的 OIDC 身份源。
type OIDCProvider struct {
	info     domain.ProviderInfo
	cfg      config.OIDCProviderConfig
	client   *http.Client
	metadata *oidcMetadata
}

type oidcMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// NewOIDCProvider 构造 OIDC 身份源；首次调用 ExchangeCode 时会拉取 discovery 文档。
func NewOIDCProvider(info domain.ProviderInfo, cfg config.OIDCProviderConfig) (*OIDCProvider, error) {
	if strings.TrimSpace(info.ID) == "" {
		return nil, fmt.Errorf("oidc provider id is required")
	}
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, fmt.Errorf("oidc provider %q: issuer is required", info.ID)
	}
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.RedirectURI) == "" {
		return nil, fmt.Errorf("oidc provider %q: client_id and redirect_uri are required", info.ID)
	}
	timeout := time.Duration(cfg.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &OIDCProvider{
		info: info,
		cfg:  cfg,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}, nil
}

func (p *OIDCProvider) Info() domain.ProviderInfo {
	if p == nil {
		return domain.ProviderInfo{}
	}
	return p.info
}

func (p *OIDCProvider) AuthorizationURL(state string) (string, error) {
	if err := p.ensureMetadata(context.Background()); err != nil {
		return "", err
	}
	oauthCfg := config.OAuth2ProviderConfig{
		AuthorizationURL: p.metadata.AuthorizationEndpoint,
		ClientID:         p.cfg.ClientID,
		RedirectURI:      p.cfg.RedirectURI,
		Scopes:           p.scopes(),
	}
	oauth := &OAuth2Provider{info: p.info, cfg: oauthCfg, client: p.client}
	return oauth.AuthorizationURL(state)
}

func (p *OIDCProvider) ExchangeCode(ctx context.Context, code string) (*domain.AuthenticatedExternalUser, error) {
	if err := p.ensureMetadata(ctx); err != nil {
		return nil, err
	}
	oauthCfg := config.OAuth2ProviderConfig{
		TokenURL:         p.metadata.TokenEndpoint,
		UserInfoURL:      p.metadata.UserinfoEndpoint,
		ClientID:         p.cfg.ClientID,
		ClientSecret:     p.cfg.ClientSecret,
		RedirectURI:      p.cfg.RedirectURI,
		SubjectClaim:     "sub",
		UsernameClaim:    p.cfg.UsernameClaim,
		DisplayNameClaim: p.cfg.DisplayNameClaim,
		EmailClaim:       p.cfg.EmailClaim,
		GroupsClaim:      p.cfg.GroupsClaim,
		TimeoutS:         p.cfg.TimeoutS,
	}
	oauth := &OAuth2Provider{info: p.info, cfg: oauthCfg, client: p.client}
	return oauth.ExchangeCode(ctx, code)
}

func (p *OIDCProvider) Extras() OAuth2ProviderExtras {
	if p == nil {
		return OAuth2ProviderExtras{}
	}
	return OAuth2ProviderExtras{
		AutoCreateUser:  p.cfg.AutoCreateUser,
		DefaultRoleCode: strings.TrimSpace(p.cfg.DefaultRoleCode),
		GroupRoleMap:    p.cfg.GroupRoleMapping,
	}
}

func (p *OIDCProvider) scopes() []string {
	if len(p.cfg.Scopes) > 0 {
		return p.cfg.Scopes
	}
	return []string{"openid", "profile", "email"}
}

func (p *OIDCProvider) ensureMetadata(ctx context.Context) error {
	if p == nil {
		return fmt.Errorf("oidc provider is nil")
	}
	if p.metadata != nil {
		return nil
	}
	issuer := strings.TrimRight(strings.TrimSpace(p.cfg.Issuer), "/")
	discoveryURL := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch oidc discovery: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("oidc discovery returned status %d", resp.StatusCode)
	}
	var meta oidcMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		return fmt.Errorf("parse oidc discovery: %w", err)
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return fmt.Errorf("oidc discovery missing required endpoints")
	}
	p.metadata = &meta
	return nil
}
