package huawei

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/734965549/aiops/internal/observability/domain"
)

// fakeAlarmStore 仅用于 auth_type=none 的测试装配；真实凭据账号禁止降级到 fake。
type fakeAlarmStore struct {
	mu     sync.Mutex
	rounds map[string]int
}

var processFakeAlarms = &fakeAlarmStore{rounds: map[string]int{}}

func (s *fakeAlarmStore) nextRound(accountID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rounds[accountID]++
	return s.rounds[accountID]
}

func (s *fakeAlarmStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rounds = map[string]int{}
}

// pullFakeAlarmRecords 返回可控的两轮告警状态：
//   - 第 1 轮：同一规则下两个资源均为 alarm
//   - 第 2 轮：资源 A 恢复（ok），资源 B 仍 alarm
//   - 第 3 轮起：两条均恢复（ok），重复拉取应全部 ignored
//
// 区域名包含 "fail" 的 scope 在第 1 轮返回 UPSTREAM_TIMEOUT，第 2 轮起成功，
// 用于 E2E 覆盖失败窗口重试。
func pullFakeAlarmRecords(pctx domain.ProviderContext, q domain.AlarmPullQuery) (*domain.AlarmPullResult, error) {
	accountID := strings.TrimSpace(pctx.Account.AccountID)
	round := processFakeAlarms.nextRound(accountID)
	now := time.Now().UTC()
	seed := shortHash(accountID)
	alarmID := "al-" + seed + "-rule"
	firstSeen := now.Add(-2 * time.Hour)
	result := &domain.AlarmPullResult{}
	for _, scope := range q.Scopes {
		scopeResult := domain.AlarmScopeResult{
			Region: scope.Region, ProjectID: scope.ProjectID,
			WindowEnd: q.RoundCutoff, Complete: true,
		}
		if strings.Contains(strings.ToLower(scope.Region), "fail") {
			if round == 1 {
				scopeResult.Complete = false
				scopeResult.WindowEnd = 0
				scopeResult.FailureCode = "UPSTREAM_TIMEOUT"
				result.Scopes = append(result.Scopes, scopeResult)
				continue
			}
			status := "alarm"
			recoveredAt := (*time.Time)(nil)
			if round >= 3 {
				status = "ok"
				recoveredAt = timePtr(now.Add(-time.Minute))
			}
			scopeResult.Records = []domain.AlarmRecord{{
				RecordID:        "ah-" + seed + "-c",
				AlarmID:         alarmID,
				Name:            "High CPU Utilization",
				Status:          status,
				Level:           "2",
				Namespace:       "SYS.ECS",
				MetricName:      "cpu_util",
				Condition:       fakeCPUCondition(),
				ResourceID:      "i-fake-" + seed + "-c",
				ResourceName:    "ecs-fake-" + seed + "-c",
				ResourceType:    "ecs",
				Dimensions:      map[string]string{"instance_id": "i-fake-" + seed + "-c"},
				RuleName:        "High CPU Utilization",
				RuleDescription: "ECS CPU utilization exceeds 90%",
				RuleType:        "MULTI_INSTANCE",
				FirstSeenAt:     &firstSeen,
				LastAlarmAt:     timePtr(now.Add(-2 * time.Minute)),
				RecoveredAt:     recoveredAt,
			}}
			result.Scopes = append(result.Scopes, scopeResult)
			continue
		}
		statusA, recoveredA := "alarm", (*time.Time)(nil)
		if round >= 2 {
			statusA = "ok"
			recoveredA = timePtr(now.Add(-time.Minute))
		}
		statusB, recoveredB := "alarm", (*time.Time)(nil)
		if round >= 3 {
			statusB = "ok"
			recoveredB = timePtr(now.Add(-time.Minute))
		}
		lastA := now.Add(-10 * time.Minute)
		if round >= 2 {
			lastA = now.Add(-20 * time.Minute)
		}
		lastB := now.Add(-10 * time.Minute)
		if round >= 2 {
			lastB = now.Add(-time.Minute)
		}
		scopeResult.Records = []domain.AlarmRecord{
			{
				RecordID:        "ah-" + seed + "-a",
				AlarmID:         alarmID,
				Name:            "High CPU Utilization",
				Status:          statusA,
				Level:           "2",
				Namespace:       "SYS.ECS",
				MetricName:      "cpu_util",
				Condition:       fakeCPUCondition(),
				ResourceID:      "i-fake-" + seed + "-a",
				ResourceName:    "ecs-fake-" + seed + "-a",
				ResourceType:    "ecs",
				Dimensions:      map[string]string{"instance_id": "i-fake-" + seed + "-a"},
				RuleName:        "High CPU Utilization",
				RuleDescription: "ECS CPU utilization exceeds 90%",
				RuleType:        "MULTI_INSTANCE",
				FirstSeenAt:     &firstSeen,
				LastAlarmAt:     &lastA,
				RecoveredAt:     recoveredA,
			},
			{
				RecordID:        "ah-" + seed + "-b",
				AlarmID:         alarmID,
				Name:            "High CPU Utilization",
				Status:          statusB,
				Level:           "2",
				Namespace:       "SYS.ECS",
				MetricName:      "cpu_util",
				Condition:       fakeCPUCondition(),
				ResourceID:      "i-fake-" + seed + "-b",
				ResourceName:    "ecs-fake-" + seed + "-b",
				ResourceType:    "ecs",
				Dimensions:      map[string]string{"instance_id": "i-fake-" + seed + "-b"},
				RuleName:        "High CPU Utilization",
				RuleDescription: "ECS CPU utilization exceeds 90%",
				RuleType:        "MULTI_INSTANCE",
				FirstSeenAt:     &firstSeen,
				LastAlarmAt:     &lastB,
				RecoveredAt:     recoveredB,
			},
		}
		result.Scopes = append(result.Scopes, scopeResult)
	}
	return result, nil
}

func shortHash(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:4])
}

func timePtr(t time.Time) *time.Time { return &t }

func fakeCPUCondition() *domain.AlarmCondition {
	return &domain.AlarmCondition{
		Filter: "average", ComparisonOperator: ">", Threshold: 90,
		Unit: "%", PeriodSeconds: 300, TriggerCount: 3, SuppressDurationS: 600,
	}
}
