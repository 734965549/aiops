package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/identity/infrastructure/identityprovider"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"go.uber.org/zap"
)

const maxLDAPImportBatch = 200

// LDAPOrganizationDTO 是对外返回的组织单元摘要。
type LDAPOrganizationDTO struct {
	DN   string `json:"dn"`
	Name string `json:"name"`
}

// LDAPDirectoryUserDTO 是对外返回的目录用户候选。
type LDAPDirectoryUserDTO struct {
	ExternalSubject  string `json:"external_subject"`
	DN               string `json:"dn,omitempty"`
	ExternalUsername string `json:"external_username"`
	DisplayName      string `json:"display_name"`
	Email            string `json:"email"`
	Imported         bool   `json:"imported"`
}

// ImportLDAPUsersInput 是从 LDAP 组织批量导入域账号的入参。
type ImportLDAPUsersInput struct {
	ProviderID       string
	OrgDN            string
	ExternalSubjects []string
	ImportAll        bool
	RoleCodes        []string
}

// ImportLDAPUsersResult 是批量导入结果摘要。
type ImportLDAPUsersResult struct {
	Created int                        `json:"created"`
	Skipped int                        `json:"skipped"`
	Failed  int                        `json:"failed"`
	Users   []ImportLDAPUserItemResult `json:"users"`
}

// ImportLDAPUserItemResult 是单条导入结果。
type ImportLDAPUserItemResult struct {
	ExternalSubject string          `json:"external_subject"`
	Status          string          `json:"status"` // created | skipped | failed
	Message         string          `json:"message,omitempty"`
	User            *CurrentUserDTO `json:"user,omitempty"`
}

// TestLDAPConnection 测试已配置 LDAP/AD 身份源的连接与绑定。
func (s *AuthService) TestLDAPConnection(ctx context.Context, providerID string) error {
	ldapProvider, err := s.requireLDAPProvider(providerID)
	if err != nil {
		return err
	}
	if err := ldapProvider.Ping(ctx); err != nil {
		return apperr.Wrap(err, apperr.CodeUnavailable, "ldap connection failed")
	}
	return nil
}

// BrowseLDAPOrganizations 浏览 LDAP/AD 下一级组织单元。
func (s *AuthService) BrowseLDAPOrganizations(ctx context.Context, providerID, parentDN string) ([]LDAPOrganizationDTO, error) {
	ldapProvider, err := s.requireLDAPProvider(providerID)
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

// PreviewLDAPUsers 预览指定组织下可导入的目录用户。
func (s *AuthService) PreviewLDAPUsers(ctx context.Context, providerID, orgDN string, limit int) ([]LDAPDirectoryUserDTO, error) {
	ldapProvider, err := s.requireLDAPProvider(providerID)
	if err != nil {
		return nil, err
	}
	return s.previewLDAPUsers(ctx, ldapProvider, providerID, orgDN, limit)
}

// ImportLDAPUsers 从已配置 LDAP 身份源批量预置域账号绑定。
func (s *AuthService) ImportLDAPUsers(ctx context.Context, in ImportLDAPUsersInput) (*ImportLDAPUsersResult, error) {
	ldapProvider, err := s.requireLDAPProvider(in.ProviderID)
	if err != nil {
		return nil, err
	}
	return s.importLDAPUsers(ctx, ldapProvider, in)
}

func (s *AuthService) previewLDAPUsers(
	ctx context.Context,
	ldapProvider *identityprovider.LDAPProvider,
	providerID, orgDN string,
	limit int,
) ([]LDAPDirectoryUserDTO, error) {
	rows, err := ldapProvider.ListDirectoryUsers(ctx, orgDN, limit)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "list ldap users failed")
	}
	out := make([]LDAPDirectoryUserDTO, 0, len(rows))
	for _, row := range rows {
		dto := LDAPDirectoryUserDTO{
			ExternalSubject:  row.ExternalSubject,
			DN:               row.DN,
			ExternalUsername: row.ExternalUsername,
			DisplayName:      row.DisplayName,
			Email:            row.Email,
		}
		if s.externalIDs != nil {
			binding, findErr := s.externalIDs.FindByProviderSubject(ctx, providerID, row.ExternalSubject)
			if findErr != nil {
				return nil, apperr.Wrap(findErr, apperr.CodeInternal, "load external identity failed")
			}
			dto.Imported = binding != nil
		}
		out = append(out, dto)
	}
	return out, nil
}

func (s *AuthService) importLDAPUsers(
	ctx context.Context,
	ldapProvider *identityprovider.LDAPProvider,
	in ImportLDAPUsersInput,
) (*ImportLDAPUsersResult, error) {
	subjects, err := s.resolveLDAPImportSubjects(ctx, ldapProvider, in)
	if err != nil {
		return nil, err
	}
	if len(subjects) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "no directory users selected for import")
	}
	if len(subjects) > maxLDAPImportBatch {
		return nil, apperr.New(apperr.CodeInvalidArgument, "too many users in one import batch")
	}
	if err := s.validateImportRoleCodes(ctx, in.RoleCodes); err != nil {
		return nil, err
	}

	result := &ImportLDAPUsersResult{Users: make([]ImportLDAPUserItemResult, 0, len(subjects))}
	for _, subject := range subjects {
		item := ImportLDAPUserItemResult{ExternalSubject: subject}
		if s.externalIDs != nil {
			binding, findErr := s.externalIDs.FindByProviderSubject(ctx, in.ProviderID, subject)
			if findErr != nil {
				return nil, apperr.Wrap(findErr, apperr.CodeInternal, "load external identity failed")
			}
			if binding != nil {
				item.Status = "skipped"
				item.Message = "already imported"
				result.Skipped++
				result.Users = append(result.Users, item)
				continue
			}
		}
		dirUser, lookupErr := ldapProvider.GetDirectoryUser(ctx, subject)
		if lookupErr != nil {
			item.Status = "failed"
			item.Message = "directory user not found"
			result.Failed++
			result.Users = append(result.Users, item)
			continue
		}
		user, provErr := s.ProvisionExternalIdentity(ctx, ProvisionExternalIdentityInput{
			ProviderID:       in.ProviderID,
			ExternalSubject:  dirUser.ExternalSubject,
			ExternalUsername: dirUser.ExternalUsername,
			DisplayName:      dirUser.DisplayName,
			Email:            dirUser.Email,
		})
		if provErr != nil {
			item.Status = "failed"
			item.Message = publicErrorMessage(provErr)
			result.Failed++
			result.Users = append(result.Users, item)
			continue
		}
		if err := s.bindUserRolesByCodes(ctx, user.ID, in.RoleCodes); err != nil {
			s.rollbackImportedExternalIdentity(ctx, user.ID, in.ProviderID, dirUser.ExternalSubject)
			item.Status = "failed"
			item.Message = publicErrorMessage(err)
			result.Failed++
			result.Users = append(result.Users, item)
			continue
		}
		item.Status = "created"
		item.User = user
		result.Created++
		result.Users = append(result.Users, item)
	}
	logger.From(ctx).Info("ldap users imported",
		zap.String("provider_id", in.ProviderID),
		zap.Int("created", result.Created),
		zap.Int("skipped", result.Skipped),
		zap.Int("failed", result.Failed),
	)
	return result, nil
}

func (s *AuthService) requireLDAPProvider(providerID string) (*identityprovider.LDAPProvider, error) {
	if s == nil || s.providers == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "external identity providers are not configured")
	}
	providerID = strings.TrimSpace(providerID)
	ldapProvider, ok := s.providers.LDAPProvider(providerID)
	if !ok {
		return nil, apperr.New(apperr.CodeInvalidArgument, "ldap/ad identity provider not found or not enabled")
	}
	return ldapProvider, nil
}

func (s *AuthService) resolveLDAPImportSubjects(
	ctx context.Context,
	ldapProvider *identityprovider.LDAPProvider,
	in ImportLDAPUsersInput,
) ([]string, error) {
	if in.ImportAll {
		rows, err := ldapProvider.ListDirectoryUsers(ctx, in.OrgDN, maxLDAPImportBatch)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeUnavailable, "list ldap users failed")
		}
		subjects := make([]string, 0, len(rows))
		for _, row := range rows {
			subjects = append(subjects, row.ExternalSubject)
		}
		return subjects, nil
	}
	subjects := make([]string, 0, len(in.ExternalSubjects))
	seen := make(map[string]struct{})
	for _, subject := range in.ExternalSubjects {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			continue
		}
		key := strings.ToLower(subject)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

func publicErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	app := apperr.FromError(err)
	if app == nil || strings.TrimSpace(app.Message) == "" {
		return err.Error()
	}
	return app.Message
}
