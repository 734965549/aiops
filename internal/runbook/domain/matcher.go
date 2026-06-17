package domain

import (
	"strings"
)

// AlertMatchContext 告警匹配上下文。
type AlertMatchContext struct {
	Name         string
	ResourceType string
	Environment  string
	Labels       map[string]string
	Annotations  map[string]string
}

// MatchResult 匹配结果与排序分。
type MatchResult struct {
	Template        Template
	StepsCount      int
	DryRunSupported bool
	MatchedReason   string
	Score           MatchScore
}

// MatchScore 匹配排序分（越大越靠前）。
type MatchScore struct {
	EnvExact      bool
	ResourceExact bool
	NameExact     bool
	NameFuzzy     bool
}

func (s MatchScore) Rank() int {
	score := 0
	if s.EnvExact {
		score += 1000
	}
	if s.ResourceExact {
		score += 100
	}
	if s.NameExact {
		score += 10
	}
	if s.NameFuzzy {
		score += 1
	}
	return score
}

// MatchesTemplate 判断模板是否匹配告警（空字段为通配）。
func MatchesTemplate(tpl Template, alert AlertMatchContext) (bool, MatchScore) {
	if !tpl.Enabled {
		return false, MatchScore{}
	}
	var score MatchScore

	if env := strings.TrimSpace(tpl.MatchEnvironment); env != "" {
		if !strings.EqualFold(env, strings.TrimSpace(alert.Environment)) {
			return false, score
		}
		score.EnvExact = true
	}
	if rt := strings.TrimSpace(tpl.MatchResourceType); rt != "" {
		if !strings.EqualFold(rt, strings.TrimSpace(alert.ResourceType)) {
			return false, score
		}
		score.ResourceExact = true
	}
	if name := strings.TrimSpace(tpl.MatchAlertName); name != "" {
		alertName := strings.TrimSpace(alert.Name)
		if strings.EqualFold(name, alertName) {
			score.NameExact = true
		} else if strings.Contains(strings.ToLower(alertName), strings.ToLower(name)) {
			score.NameFuzzy = true
		} else {
			return false, score
		}
	}
	return true, score
}

// SupportsDryRun 模板是否至少有一个步骤支持 dry-run。
func SupportsDryRun(steps []Step) bool {
	for _, s := range steps {
		if s.DryRunSupported {
			return true
		}
	}
	return false
}
