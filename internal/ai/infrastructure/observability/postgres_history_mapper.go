package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
)

func mapPostgresSession(item rdsmodel.Session) aiapp.PostgresSession {
	return aiapp.PostgresSession{
		SampleTime:               stringValue(item.SampleTime),
		BlockedProcessID:         truncatePostgresText(stringValue(item.BlockedProcessId), 512),
		DatabaseName:             truncatePostgresText(stringValue(item.DatabaseName), 128),
		SessionID:                int32Value(item.SessionId),
		UserHash:                 hashPostgresIdentity(stringValue(item.UserName), "u"),
		AppName:                  truncatePostgresText(stringValue(item.AppName), 256),
		ClientAddress:            maskPostgresClientAddress(stringValue(item.ClientIpAddress)),
		SessionStartTime:         stringValue(item.SessionStartTime),
		TransactionStartTime:     stringValue(item.TransactionStartTime),
		TransactionExecutionTime: int32Value(item.TransactionExecutionTime),
		QueryStartTime:           stringValue(item.QueryStartTime),
		StateChangeTime:          stringValue(item.StateChangeTime),
		WaitEventType:            truncatePostgresText(stringValue(item.WaitEventType), 128),
		WaitEventName:            truncatePostgresText(stringValue(item.WaitEventName), 128),
		SessionStatus:            truncatePostgresText(stringValue(item.SessionStatus), 64),
		QueryID:                  truncatePostgresText(stringValue(item.QueryId), 128),
		SQLHash:                  hashPostgresSQL(stringValue(item.SqlStatement)),
		ProcessType:              truncatePostgresText(stringValue(item.ProcessType), 128),
	}
}

func mapPostgresWaitEvent(item rdsmodel.Event) aiapp.PostgresWaitEvent {
	return aiapp.PostgresWaitEvent{
		SampleTime: stringValue(item.SampleTime), Count: int32Value(item.Count),
		SessionStatus: truncatePostgresText(stringValue(item.SessionStatus), 64),
		WaitEventType: truncatePostgresText(stringValue(item.WaitEventType), 128),
		WaitEventName: truncatePostgresText(stringValue(item.WaitEventName), 128),
	}
}

func mapPostgresTopSQL(item rdsmodel.TopSql) aiapp.PostgresTopSQL {
	return aiapp.PostgresTopSQL{
		SampleTime: stringValue(item.SampleTime), Count: int32Value(item.Count),
		DatabaseName: truncatePostgresText(stringValue(item.DatabaseName), 128),
		UserHash:     hashPostgresIdentity(stringValue(item.UserName), "u"),
		QueryID:      truncatePostgresText(stringValue(item.QueryId), 128),
		SQLHash:      hashPostgresSQL(stringValue(item.SqlStatement)),
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func int32Value(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func hashPostgresIdentity(value, prefix string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return prefix + ":" + hex.EncodeToString(sum[:])[:16]
}

func hashPostgresSQL(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	return hashPostgresIdentity(value, "sql")
}

func maskPostgresClientAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return hashPostgresIdentity(value, "client")
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ipv4[3] = 0
		return ipv4.String() + "/24"
	}
	for i := 8; i < 16; i++ {
		ip[i] = 0
	}
	return ip.String() + "/64"
}

func truncatePostgresText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
