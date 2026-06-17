package application

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/734965549/aiops/internal/asset/domain"
)

// MatchInput Alert 接入时用于匹配 application_id / resource_id 的标签快照。
type MatchInput struct {
	SourceType      string
	ApplicationName string
	ResourceName    string
	ResourceType    string
	Environment     string
	Labels          map[string]string
}

// MatchResult 匹配结果；未匹配时 ID 为空，Alert 仍保存告警。
type MatchResult struct {
	ApplicationID string
	ResourceID    string
}

// MatcherService 按用户配置规则与 §9.1 默认规则匹配已注册应用与资源。
type MatcherService struct {
	apps      domain.ApplicationRepository
	resources domain.ResourceRepository
	rules     domain.MatchRuleRepository
}

// NewMatcherService 构造匹配服务。
func NewMatcherService(apps domain.ApplicationRepository, resources domain.ResourceRepository, rules domain.MatchRuleRepository) *MatcherService {
	return &MatcherService{apps: apps, resources: resources, rules: rules}
}

// Match 尝试将告警标签关联到 Asset 注册表；失败返回空结果，不报错。
func (s *MatcherService) Match(ctx context.Context, in MatchInput) (MatchResult, error) {
	if s == nil || s.apps == nil {
		return MatchResult{}, nil
	}
	if out, ok := s.matchByRules(ctx, in); ok {
		return out, nil
	}
	return s.matchDefault(ctx, in)
}

func (s *MatcherService) matchByRules(ctx context.Context, in MatchInput) (MatchResult, bool) {
	if s == nil || s.rules == nil {
		return MatchResult{}, false
	}
	rows, err := s.rules.ListEnabledByPriority(ctx)
	if err != nil || len(rows) == 0 {
		return MatchResult{}, false
	}
	sourceType := strings.TrimSpace(in.SourceType)
	for _, rule := range rows {
		if !ruleMatchesSource(rule, sourceType) {
			continue
		}
		labelVal := labelValue(in.Labels, rule.LabelKey)
		if labelVal == "" || !matchLabelPattern(rule.LabelValuePattern, labelVal) {
			continue
		}
		out := MatchResult{ApplicationID: rule.ApplicationID}
		if rule.TargetType == domain.TargetResource && strings.TrimSpace(rule.ResourceID) != "" {
			out.ResourceID = rule.ResourceID
		}
		return out, true
	}
	return MatchResult{}, false
}

func (s *MatcherService) matchDefault(ctx context.Context, in MatchInput) (MatchResult, error) {
	appName := firstNonEmpty(
		in.ApplicationName,
		labelValue(in.Labels, "service"),
		labelValue(in.Labels, "app"),
		labelValue(in.Labels, "application"),
	)
	if appName == "" {
		return MatchResult{}, nil
	}
	app, err := s.apps.FindByNameEnv(ctx, appName, in.Environment)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return MatchResult{}, nil
		}
		return MatchResult{}, wrapAssetError(err, "match application failed")
	}
	out := MatchResult{ApplicationID: app.ID}
	if s.resources == nil {
		return out, nil
	}
	res, err := s.resources.FindBestMatch(ctx, domain.ResourceMatchQuery{
		ApplicationID: app.ID,
		Name:          in.ResourceName,
		ResourceType:  in.ResourceType,
		Namespace:     labelValue(in.Labels, "namespace"),
		Pod:           labelValue(in.Labels, "pod"),
		Node:          labelValue(in.Labels, "node"),
		Instance:      firstNonEmpty(labelValue(in.Labels, "instance"), in.ResourceName),
	})
	if err == nil && res != nil {
		out.ResourceID = res.ID
	}
	return out, nil
}

func ruleMatchesSource(rule domain.MatchRule, sourceType string) bool {
	if rule.SourceType == domain.MatchSourceAll || string(rule.SourceType) == "" {
		return true
	}
	return strings.EqualFold(string(rule.SourceType), sourceType)
}

func matchLabelPattern(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" || value == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	matched, err := path.Match(pattern, value)
	return err == nil && matched
}
