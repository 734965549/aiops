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
	operation, summary := summarizePostgresSQL(stringValue(item.SqlStatement))
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
		SQLOperation:             operation,
		SQLSummary:               summary,
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
	operation, summary := summarizePostgresSQL(stringValue(item.SqlStatement))
	return aiapp.PostgresTopSQL{
		SampleTime: stringValue(item.SampleTime), Count: int32Value(item.Count),
		DatabaseName: truncatePostgresText(stringValue(item.DatabaseName), 128),
		UserHash:     hashPostgresIdentity(stringValue(item.UserName), "u"),
		QueryID:      truncatePostgresText(stringValue(item.QueryId), 128),
		SQLHash:      hashPostgresSQL(stringValue(item.SqlStatement)),
		SQLOperation: operation,
		SQLSummary:   summary,
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

func summarizePostgresSQL(raw string) (string, string) {
	sanitized := strings.Join(strings.Fields(sanitizePostgresSQL(raw)), " ")
	return postgresSQLOperation(sanitized), truncatePostgresText(sanitized, 300)
}

func sanitizePostgresSQL(raw string) string {
	var out strings.Builder
	out.Grow(len(raw))
	for i := 0; i < len(raw); {
		if prefix := postgresStringPrefixLen(raw, i); prefix >= 0 {
			out.WriteByte('?')
			i = skipPostgresSingleQuoted(raw, i+prefix)
			continue
		}
		if raw[i] == '"' {
			end, ok := postgresDoubleQuotedEnd(raw, i)
			if !ok {
				out.WriteString(`"?"`)
				break
			}
			out.WriteString(raw[i:end])
			i = end
			continue
		}
		if i+1 < len(raw) && raw[i] == '-' && raw[i+1] == '-' {
			out.WriteByte(' ')
			i += 2
			for i < len(raw) && raw[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(raw) && raw[i] == '/' && raw[i+1] == '*' {
			out.WriteByte(' ')
			i = skipPostgresBlockComment(raw, i)
			continue
		}
		if raw[i] == '$' {
			if tag, ok := postgresDollarQuoteTag(raw, i); ok {
				out.WriteByte('?')
				body := i + len(tag)
				end := strings.Index(raw[body:], tag)
				if end < 0 {
					break
				}
				i = body + end + len(tag)
				continue
			}
		}
		if isPostgresDigit(raw[i]) && (i == 0 || !isPostgresIdentifierPart(raw[i-1])) {
			out.WriteByte('?')
			i = scanPostgresNumber(raw, i)
			continue
		}
		if raw[i] == '.' && i+1 < len(raw) && isPostgresDigit(raw[i+1]) && (i == 0 || !isPostgresIdentifierPart(raw[i-1])) {
			out.WriteByte('?')
			i = scanPostgresNumber(raw, i)
			continue
		}
		out.WriteByte(raw[i])
		i++
	}
	return out.String()
}

func postgresStringPrefixLen(raw string, start int) int {
	if raw[start] == '\'' {
		return 0
	}
	if start+1 < len(raw) && strings.ContainsRune("EeBbXxNn", rune(raw[start])) && raw[start+1] == '\'' {
		return 1
	}
	if start+2 < len(raw) && (raw[start] == 'U' || raw[start] == 'u') && raw[start+1] == '&' && raw[start+2] == '\'' {
		return 2
	}
	return -1
}

func skipPostgresSingleQuoted(raw string, quote int) int {
	for i := quote + 1; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			i++
			continue
		}
		if raw[i] != '\'' {
			continue
		}
		if i+1 < len(raw) && raw[i+1] == '\'' {
			i++
			continue
		}
		return i + 1
	}
	return len(raw)
}

func postgresDoubleQuotedEnd(raw string, quote int) (int, bool) {
	for i := quote + 1; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		if i+1 < len(raw) && raw[i+1] == '"' {
			i++
			continue
		}
		return i + 1, true
	}
	return len(raw), false
}

func skipPostgresBlockComment(raw string, start int) int {
	depth := 1
	for i := start + 2; i < len(raw); {
		if i+1 < len(raw) && raw[i] == '/' && raw[i+1] == '*' {
			depth++
			i += 2
			continue
		}
		if i+1 < len(raw) && raw[i] == '*' && raw[i+1] == '/' {
			depth--
			i += 2
			if depth == 0 {
				return i
			}
			continue
		}
		i++
	}
	return len(raw)
}

func postgresDollarQuoteTag(raw string, start int) (string, bool) {
	for i := start + 1; i < len(raw); i++ {
		if raw[i] == '$' {
			return raw[start : i+1], true
		}
		if raw[i] != '_' && !isPostgresLetter(raw[i]) && (i == start+1 || !isPostgresDigit(raw[i])) {
			return "", false
		}
	}
	return "", false
}

func scanPostgresNumber(raw string, start int) int {
	i := start
	if i+2 < len(raw) && raw[i] == '0' && (raw[i+1] == 'x' || raw[i+1] == 'X') {
		i += 2
		for i < len(raw) && (isPostgresDigit(raw[i]) || raw[i] >= 'a' && raw[i] <= 'f' || raw[i] >= 'A' && raw[i] <= 'F') {
			i++
		}
		return i
	}
	if raw[i] == '.' {
		i++
	}
	for i < len(raw) && isPostgresDigit(raw[i]) {
		i++
	}
	if i < len(raw) && raw[i] == '.' {
		i++
		for i < len(raw) && isPostgresDigit(raw[i]) {
			i++
		}
	}
	if i < len(raw) && (raw[i] == 'e' || raw[i] == 'E') {
		end := i + 1
		if end < len(raw) && (raw[end] == '+' || raw[end] == '-') {
			end++
		}
		digits := end
		for end < len(raw) && isPostgresDigit(raw[end]) {
			end++
		}
		if end > digits {
			i = end
		}
	}
	return i
}

func postgresSQLOperation(sql string) string {
	depth := 0
	for i := 0; i < len(sql); {
		switch sql[i] {
		case '(':
			depth++
			i++
			continue
		case ')':
			if depth > 0 {
				depth--
			}
			i++
			continue
		case '"':
			end, ok := postgresDoubleQuotedEnd(sql, i)
			if !ok {
				return ""
			}
			i = end
			continue
		}
		if !isPostgresLetter(sql[i]) {
			i++
			continue
		}
		end := i + 1
		for end < len(sql) && isPostgresIdentifierPart(sql[end]) {
			end++
		}
		if depth == 0 {
			switch strings.ToUpper(sql[i:end]) {
			case "SELECT", "INSERT", "UPDATE", "DELETE", "MERGE", "CALL", "CREATE", "ALTER", "DROP",
				"TRUNCATE", "REINDEX", "VACUUM", "ANALYZE", "COPY", "SET", "SHOW", "BEGIN", "START",
				"COMMIT", "ROLLBACK", "EXPLAIN", "GRANT", "REVOKE":
				return strings.ToUpper(sql[i:end])
			}
		}
		i = end
	}
	return ""
}

func isPostgresLetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isPostgresDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func isPostgresIdentifierPart(value byte) bool {
	return isPostgresLetter(value) || isPostgresDigit(value) || value == '_' || value == '$' || value >= 0x80
}
