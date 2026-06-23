package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/internal/identity/infrastructure/identityprovider"
	"github.com/734965549/aiops/internal/identity/infrastructure/ldapsession"
	"github.com/734965549/aiops/pkg/config"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/google/uuid"
)

// LDAPConnectionInput 是管理员在前端填写的 LDAP/AD 连接参数。
type LDAPConnectionInput struct {
	ProviderID         string
	Type               string
	ServerURL          string
	BindDN             string
	BindPassword       string
	BaseDN             string
	StartTLS           bool
	InsecureSkipVerify bool
	BrowseOrgFilter    string
	BrowseUserFilter   string
	AttrSubject        string
}

// LDAPConnectResult 是建立 LDAP 浏览会话的返回。
type LDAPConnectResult struct {
	SessionID  string `json:"session_id"`
	ProviderID string `json:"provider_id"`
	BaseDN     string `json:"base_dn"`
	ExpiresIn  int    `json:"expires_in"`
}

// ConnectLDAPSession 使用管理员填写的连接参数建立短期浏览会话。
func (s *AuthService) ConnectLDAPSession(ctx context.Context, adminUserID string, in LDAPConnectionInput) (*LDAPConnectResult, error) {
	if s == nil || s.ldapBrowseStore == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ldap browse session is not configured")
	}
	providerID, err := validateProviderID(in.ProviderID)
	if err != nil {
		return nil, err
	}
	ldapCfg, err := toLDAPProviderConfig(in)
	if err != nil {
		return nil, err
	}
	ldapProvider, err := identityprovider.BuildLDAPProviderFromConnection(s.appEnv, providerID, in.Type, ldapCfg)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInvalidArgument, "invalid ldap connection")
	}
	if err := ldapProvider.Ping(ctx); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "ldap connection failed")
	}
	session := ldapsession.Session{
		ID:         uuid.NewString(),
		ProviderID: providerID,
		Type:       strings.TrimSpace(in.Type),
		LDAP:       ldapCfg,
		UserID:     strings.TrimSpace(adminUserID),
		CreatedAt:  time.Now(),
	}
	if err := s.ldapBrowseStore.Create(ctx, session, ldapsession.DefaultTTL); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "create ldap browse session failed")
	}
	return &LDAPConnectResult{
		SessionID:  session.ID,
		ProviderID: providerID,
		BaseDN:     ldapCfg.BaseDN,
		ExpiresIn:  int(ldapsession.DefaultTTL.Seconds()),
	}, nil
}

// CloseLDAPSession 关闭 LDAP 浏览会话。
func (s *AuthService) CloseLDAPSession(ctx context.Context, adminUserID, sessionID string) error {
	if s == nil || s.ldapBrowseStore == nil {
		return apperr.New(apperr.CodeUnavailable, "ldap browse session is not configured")
	}
	return s.ldapBrowseStore.Delete(ctx, strings.TrimSpace(sessionID), strings.TrimSpace(adminUserID))
}

// BrowseLDAPSessionOrganizations 浏览会话下的一级组织单元。
func (s *AuthService) BrowseLDAPSessionOrganizations(ctx context.Context, adminUserID, sessionID, parentDN string) ([]LDAPOrganizationDTO, error) {
	ldapProvider, _, err := s.requireLDAPSessionProvider(ctx, adminUserID, sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := ldapProvider.BrowseOrganizations(ctx, parentDN)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "browse ldap organizations failed")
	}
	out := make([]LDAPOrganizationDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, LDAPOrganizationDTO{DN: row.DN, Name: row.Name})
	}
	return out, nil
}

// PreviewLDAPSessionUsers 预览会话下指定组织的目录用户。
func (s *AuthService) PreviewLDAPSessionUsers(ctx context.Context, adminUserID, sessionID, orgDN string, limit int) ([]LDAPDirectoryUserDTO, error) {
	ldapProvider, providerID, err := s.requireLDAPSessionProvider(ctx, adminUserID, sessionID)
	if err != nil {
		return nil, err
	}
	return s.previewLDAPUsers(ctx, ldapProvider, providerID, orgDN, limit)
}

// ImportLDAPSessionUsers 从会话连接的目录批量导入用户并绑定角色。
func (s *AuthService) ImportLDAPSessionUsers(ctx context.Context, adminUserID, sessionID string, in ImportLDAPUsersInput) (*ImportLDAPUsersResult, error) {
	ldapProvider, providerID, err := s.requireLDAPSessionProvider(ctx, adminUserID, sessionID)
	if err != nil {
		return nil, err
	}
	in.ProviderID = providerID
	return s.importLDAPUsers(ctx, ldapProvider, in)
}

func (s *AuthService) requireLDAPSessionProvider(ctx context.Context, adminUserID, sessionID string) (*identityprovider.LDAPProvider, string, error) {
	if s == nil || s.ldapBrowseStore == nil {
		return nil, "", apperr.New(apperr.CodeUnavailable, "ldap browse session is not configured")
	}
	session, err := s.ldapBrowseStore.Get(ctx, strings.TrimSpace(sessionID), strings.TrimSpace(adminUserID))
	if err != nil {
		return nil, "", wrapIdentityOpError(err, "load ldap browse session failed")
	}
	ldapProvider, err := identityprovider.BuildLDAPProviderFromConnection(s.appEnv, session.ProviderID, session.Type, session.LDAP)
	if err != nil {
		return nil, "", apperr.Wrap(err, apperr.CodeInternal, "build ldap provider failed")
	}
	return ldapProvider, session.ProviderID, nil
}

func toLDAPProviderConfig(in LDAPConnectionInput) (config.LDAPProviderConfig, error) {
	serverURL := strings.TrimSpace(in.ServerURL)
	baseDN := strings.TrimSpace(in.BaseDN)
	if serverURL == "" || baseDN == "" {
		return config.LDAPProviderConfig{}, apperr.New(apperr.CodeInvalidArgument, "server_url and base_dn are required")
	}
	return config.LDAPProviderConfig{
		ServerURL:          serverURL,
		BindDN:             strings.TrimSpace(in.BindDN),
		BindPassword:       in.BindPassword,
		BaseDN:             baseDN,
		StartTLS:           in.StartTLS,
		InsecureSkipVerify: in.InsecureSkipVerify,
		BrowseOrgFilter:    strings.TrimSpace(in.BrowseOrgFilter),
		BrowseUserFilter:   strings.TrimSpace(in.BrowseUserFilter),
		AttrSubject:        strings.TrimSpace(in.AttrSubject),
		TimeoutS:           10,
	}, nil
}

func validateProviderID(providerID string) (string, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return "", apperr.New(apperr.CodeInvalidArgument, "provider_id is required")
	}
	if len(providerID) > 64 {
		return "", apperr.New(apperr.CodeInvalidArgument, "provider_id is too long")
	}
	for _, r := range providerID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return "", apperr.New(apperr.CodeInvalidArgument, "provider_id contains invalid characters")
	}
	return providerID, nil
}

func (s *AuthService) bindUserRolesByCodes(ctx context.Context, userID string, roleCodes []string) error {
	if s == nil || s.ac == nil || len(roleCodes) == 0 {
		return nil
	}
	for _, code := range roleCodes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		role, err := s.ac.FindRoleByCode(ctx, code)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "find role failed")
		}
		if role == nil {
			return apperr.New(apperr.CodeInvalidArgument, "role not found: "+code)
		}
		has, err := s.ac.HasUserRole(ctx, userID, role.ID)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "check user role failed")
		}
		if has {
			continue
		}
		if err := s.ac.BindUserRole(ctx, userID, role.ID, domain.UserRoleSourceLDAPImport); err != nil {
			if errors.Is(err, domain.ErrReferenceNotFound) {
				return apperr.New(apperr.CodeInvalidArgument, "role not found: "+code)
			}
			return apperr.Wrap(err, apperr.CodeInternal, "bind user role failed")
		}
	}
	return nil
}

func (s *AuthService) validateImportRoleCodes(ctx context.Context, roleCodes []string) error {
	if s == nil || s.ac == nil || len(roleCodes) == 0 {
		return nil
	}
	for _, code := range roleCodes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		role, err := s.ac.FindRoleByCode(ctx, code)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "find role failed")
		}
		if role == nil {
			return apperr.New(apperr.CodeInvalidArgument, "role not found: "+code)
		}
	}
	return nil
}

func (s *AuthService) rollbackImportedExternalIdentity(ctx context.Context, userID, providerID, externalSubject string) {
	if s == nil {
		return
	}
	if s.ac != nil && strings.TrimSpace(userID) != "" {
		bindings, err := s.ac.ListUserRoleBindings(ctx, userID)
		if err != nil {
			logger.From(ctx).Warn("rollback imported roles list failed", logger.String("user_id", userID), logger.Error(err))
		} else {
			for _, b := range bindings {
				if b.Source != domain.UserRoleSourceLDAPImport {
					continue
				}
				if err := s.ac.UnbindUserRole(ctx, userID, b.RoleID); err != nil {
					logger.From(ctx).Warn("rollback imported role unbind failed",
						logger.String("user_id", userID),
						logger.String("role_id", b.RoleID),
						logger.Error(err),
					)
				}
			}
		}
	}
	if s.externalIDs != nil {
		if err := s.externalIDs.DeleteByProviderSubject(ctx, providerID, externalSubject); err != nil {
			logger.From(ctx).Warn("rollback external identity failed",
				logger.String("provider_id", providerID),
				logger.String("external_subject", externalSubject),
				logger.Error(err),
			)
		}
	}
	s.rollbackCreatedUser(ctx, userID)
}
