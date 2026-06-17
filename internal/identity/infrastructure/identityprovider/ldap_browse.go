package identityprovider

import (
	"context"
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/go-ldap/ldap/v3"
)

const (
	defaultLDAPOrgFilter        = "(|(objectClass=organizationalUnit)(objectClass=container)(objectClass=organization))"
	defaultLDAPBrowseUserFilter = "(|(objectClass=inetOrgPerson)(objectClass=person)(objectClass=organizationalPerson))"
	defaultADBrowseUserFilter   = "(&(objectCategory=person)(objectClass=user))"
	maxLDAPBrowsePageSize       = 200
)

// LDAPOrganization 表示 LDAP/AD 目录中的一个组织单元。
type LDAPOrganization struct {
	DN   string `json:"dn"`
	Name string `json:"name"`
}

// LDAPDirectoryUser 是目录中可用于导入的平台用户候选。
type LDAPDirectoryUser struct {
	ExternalSubject  string `json:"external_subject"`
	DN               string `json:"dn"`
	ExternalUsername string `json:"external_username"`
	DisplayName      string `json:"display_name"`
	Email            string `json:"email"`
}

// BrowseOrganizations 列出 parentDN 下一级组织单元；parentDN 为空时使用配置的 base_dn。
func (p *LDAPProvider) BrowseOrganizations(_ context.Context, parentDN string) ([]LDAPOrganization, error) {
	if p == nil {
		return nil, fmt.Errorf("ldap provider is nil")
	}
	baseDN := strings.TrimSpace(parentDN)
	if baseDN == "" {
		baseDN = strings.TrimSpace(p.cfg.BaseDN)
	}
	if baseDN == "" {
		return nil, fmt.Errorf("parent dn is required")
	}

	conn, err := p.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := p.serviceBind(conn); err != nil {
		return nil, err
	}

	orgFilter := strings.TrimSpace(p.cfg.BrowseOrgFilter)
	if orgFilter == "" {
		orgFilter = defaultLDAPOrgFilter
	}
	searchReq := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		orgFilter,
		[]string{"dn", "ou", "cn", "name", "description"},
		nil,
	)
	result, err := conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("ldap search organizations: %w", err)
	}
	out := make([]LDAPOrganization, 0, len(result.Entries))
	for _, entry := range result.Entries {
		name := firstAttr(entry, "ou", "name", "cn", "description")
		if name == "" {
			name = entry.DN
		}
		out = append(out, LDAPOrganization{DN: entry.DN, Name: name})
	}
	return out, nil
}

// ListDirectoryUsers 列出 orgDN 下（含子树）符合 browse 过滤条件的用户，最多 limit 条。
func (p *LDAPProvider) ListDirectoryUsers(_ context.Context, orgDN string, limit int) ([]LDAPDirectoryUser, error) {
	if p == nil {
		return nil, fmt.Errorf("ldap provider is nil")
	}
	orgDN = strings.TrimSpace(orgDN)
	if orgDN == "" {
		orgDN = strings.TrimSpace(p.cfg.BaseDN)
	}
	if orgDN == "" {
		return nil, fmt.Errorf("org dn is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > maxLDAPBrowsePageSize {
		limit = maxLDAPBrowsePageSize
	}

	conn, err := p.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := p.serviceBind(conn); err != nil {
		return nil, err
	}

	searchReq := ldap.NewSearchRequest(
		orgDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, limit, 0, false,
		p.browseUserFilter(),
		p.directoryUserAttributes(),
		nil,
	)
	result, err := conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("ldap search users: %w", err)
	}
	out := make([]LDAPDirectoryUser, 0, len(result.Entries))
	for _, entry := range result.Entries {
		out = append(out, p.entryToDirectoryUser(entry))
	}
	return out, nil
}

// GetDirectoryUser 按 external_subject（稳定主体或历史 DN）读取单个目录用户（导入前二次校验）。
func (p *LDAPProvider) GetDirectoryUser(_ context.Context, externalSubject string) (*LDAPDirectoryUser, error) {
	if p == nil {
		return nil, fmt.Errorf("ldap provider is nil")
	}
	externalSubject = strings.TrimSpace(externalSubject)
	if externalSubject == "" {
		return nil, fmt.Errorf("external subject is required")
	}
	if looksLikeDN(externalSubject) {
		if user, err := p.getDirectoryUserByDN(externalSubject); err == nil {
			return user, nil
		}
	}
	return p.getDirectoryUserBySubject(externalSubject)
}

func (p *LDAPProvider) getDirectoryUserByDN(dn string) (*LDAPDirectoryUser, error) {
	conn, err := p.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := p.serviceBind(conn); err != nil {
		return nil, err
	}
	searchReq := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		p.browseUserFilter(),
		p.directoryUserAttributes(),
		nil,
	)
	result, err := conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("ldap lookup user: %w", err)
	}
	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("directory user not found")
	}
	user := p.entryToDirectoryUser(result.Entries[0])
	return &user, nil
}

func (p *LDAPProvider) getDirectoryUserBySubject(subject string) (*LDAPDirectoryUser, error) {
	conn, err := p.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := p.serviceBind(conn); err != nil {
		return nil, err
	}
	baseDN := strings.TrimSpace(p.cfg.BaseDN)
	if baseDN == "" {
		return nil, fmt.Errorf("base dn is required")
	}
	userFilter := p.browseUserFilter()
	for _, attr := range p.subjectAttributeCandidates() {
		attrFilter, err := buildSubjectEqualsFilter(attr, subject)
		if err != nil {
			continue
		}
		combined := fmt.Sprintf("(&%s%s)", userFilter, attrFilter)
		searchReq := ldap.NewSearchRequest(
			baseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 0, false,
			combined,
			p.directoryUserAttributes(),
			nil,
		)
		result, err := conn.Search(searchReq)
		if err != nil {
			continue
		}
		for _, entry := range result.Entries {
			if p.entryExternalSubject(entry) != subject {
				continue
			}
			user := p.entryToDirectoryUser(entry)
			return &user, nil
		}
	}
	return nil, fmt.Errorf("directory user not found")
}

// Ping 使用服务账号测试 LDAP 连接与绑定。
func (p *LDAPProvider) Ping(_ context.Context) error {
	if p == nil {
		return fmt.Errorf("ldap provider is nil")
	}
	conn, err := p.connect()
	if err != nil {
		return err
	}
	defer conn.Close()
	return p.serviceBind(conn)
}

func (p *LDAPProvider) serviceBind(conn ldapConn) error {
	bindDN := strings.TrimSpace(p.cfg.BindDN)
	if bindDN == "" {
		return nil
	}
	if err := conn.Bind(bindDN, p.cfg.BindPassword); err != nil {
		return fmt.Errorf("ldap service bind failed: %w", err)
	}
	return nil
}

func (p *LDAPProvider) browseUserFilter() string {
	if f := strings.TrimSpace(p.cfg.BrowseUserFilter); f != "" {
		return f
	}
	if p.info.Type == domain.ProviderTypeAD {
		return defaultADBrowseUserFilter
	}
	return defaultLDAPBrowseUserFilter
}

func (p *LDAPProvider) directoryUserAttributes() []string {
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
	add("uid")
	add("sAMAccountName")
	add("userPrincipalName")
	add(p.cfg.AttrDisplayName)
	add("cn")
	add("displayName")
	add(p.cfg.AttrEmail)
	add("mail")
	p.addSubjectAttributes(add)
	return attrs
}

func (p *LDAPProvider) entryToDirectoryUser(entry *ldap.Entry) LDAPDirectoryUser {
	if entry == nil {
		return LDAPDirectoryUser{}
	}
	loginName := p.entryLoginName(entry)
	displayName := firstAttr(entry, p.cfg.AttrDisplayName, "displayName", "cn")
	if displayName == "" {
		displayName = loginName
	}
	email := firstAttr(entry, p.cfg.AttrEmail, "mail", "userPrincipalName")
	if loginName == "" {
		loginName = email
	}
	return LDAPDirectoryUser{
		ExternalSubject:  p.entryExternalSubject(entry),
		DN:               entry.DN,
		ExternalUsername: loginName,
		DisplayName:      displayName,
		Email:            email,
	}
}

func (p *LDAPProvider) entryLoginName(entry *ldap.Entry) string {
	if entry == nil {
		return ""
	}
	if p.info.Type == domain.ProviderTypeAD {
		if v := strings.TrimSpace(entry.GetAttributeValue("sAMAccountName")); v != "" {
			return v
		}
		if v := strings.TrimSpace(entry.GetAttributeValue("userPrincipalName")); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(entry.GetAttributeValue("uid")); v != "" {
		return v
	}
	if v := strings.TrimSpace(entry.GetAttributeValue("mail")); v != "" {
		return v
	}
	return firstAttr(entry, "cn")
}
