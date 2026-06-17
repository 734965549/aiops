package identityprovider

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
)

func (p *LDAPProvider) subjectAttributeCandidates() []string {
	if p == nil {
		return nil
	}
	if attr := strings.TrimSpace(p.cfg.AttrSubject); attr != "" {
		return []string{attr}
	}
	if p.info.Type == domain.ProviderTypeAD {
		return []string{"objectGUID", "objectSid"}
	}
	return []string{"entryUUID"}
}

func (p *LDAPProvider) addSubjectAttributes(add func(string)) {
	for _, attr := range p.subjectAttributeCandidates() {
		add(attr)
	}
}

// entryExternalSubject 从目录条目解析稳定外部主体；无法解析时回退为 DN。
func (p *LDAPProvider) entryExternalSubject(entry *ldap.Entry) string {
	if entry == nil {
		return ""
	}
	for _, attr := range p.subjectAttributeCandidates() {
		if v := normalizeLDAPSubjectAttribute(attr, entry.GetRawAttributeValue(attr)); v != "" {
			return v
		}
	}
	return strings.TrimSpace(entry.DN)
}

func normalizeLDAPSubjectAttribute(attr string, raw []byte) string {
	attr = strings.TrimSpace(attr)
	if attr == "" || len(raw) == 0 {
		return ""
	}
	switch {
	case strings.EqualFold(attr, "objectGUID"):
		return formatObjectGUID(raw)
	case strings.EqualFold(attr, "objectSid"):
		return formatObjectSID(raw)
	default:
		return strings.TrimSpace(string(raw))
	}
}

func formatObjectGUID(raw []byte) string {
	if len(raw) != 16 {
		return strings.ToLower(hex.EncodeToString(raw))
	}
	// AD 以混合字节序存储 GUID，转换为 RFC 4122 再规范化。
	data := make([]byte, 16)
	data[0], data[1], data[2], data[3] = raw[3], raw[2], raw[1], raw[0]
	data[4], data[5] = raw[5], raw[4]
	data[6], data[7] = raw[7], raw[6]
	copy(data[8:], raw[8:])
	u, err := uuid.FromBytes(data)
	if err != nil {
		return strings.ToLower(hex.EncodeToString(raw))
	}
	return strings.ToLower(u.String())
}

func formatObjectSID(raw []byte) string {
	if len(raw) < 8 {
		return ""
	}
	var b strings.Builder
	b.WriteString("S-")
	b.WriteString(strconv.Itoa(int(raw[0])))
	authority := int64(raw[2])<<40 | int64(raw[3])<<32 | int64(raw[4])<<24 |
		int64(raw[5])<<16 | int64(raw[6])<<8 | int64(raw[7])
	b.WriteString("-")
	b.WriteString(strconv.FormatInt(authority, 10))
	subCount := int(raw[1])
	offset := 8
	for i := 0; i < subCount; i++ {
		if offset+4 > len(raw) {
			break
		}
		sub := uint32(raw[offset]) |
			uint32(raw[offset+1])<<8 |
			uint32(raw[offset+2])<<16 |
			uint32(raw[offset+3])<<24
		b.WriteString("-")
		b.WriteString(strconv.FormatUint(uint64(sub), 10))
		offset += 4
	}
	return b.String()
}

func looksLikeDN(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !strings.Contains(value, "=") {
		return false
	}
	upper := strings.ToUpper(value)
	return strings.Contains(value, ",") ||
		strings.HasPrefix(upper, "CN=") ||
		strings.HasPrefix(upper, "UID=") ||
		strings.HasPrefix(upper, "OU=") ||
		strings.HasPrefix(upper, "DC=")
}

func buildSubjectEqualsFilter(attr, value string) (string, error) {
	attr = strings.TrimSpace(attr)
	value = strings.TrimSpace(value)
	if attr == "" || value == "" {
		return "", fmt.Errorf("subject filter requires attr and value")
	}
	switch {
	case strings.EqualFold(attr, "objectGUID"):
		data, err := guidStringToADBytes(value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s=%s)", attr, encodeBinaryLDAPFilter(data)), nil
	case strings.EqualFold(attr, "objectSid"):
		data, err := parseObjectSIDString(value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s=%s)", attr, encodeBinaryLDAPFilter(data)), nil
	default:
		return fmt.Sprintf("(%s=%s)", attr, ldap.EscapeFilter(value)), nil
	}
}

func guidStringToADBytes(value string) ([]byte, error) {
	u, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	b := u[:]
	out := make([]byte, 16)
	out[0], out[1], out[2], out[3] = b[3], b[2], b[1], b[0]
	out[4], out[5] = b[5], b[4]
	out[6], out[7] = b[7], b[6]
	copy(out[8:], b[8:])
	return out, nil
}

func parseObjectSIDString(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToUpper(value), "S-") {
		return nil, fmt.Errorf("invalid sid")
	}
	parts := strings.Split(value[2:], "-")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid sid")
	}
	revision, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, err
	}
	authority, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	sid := make([]byte, 8+4*len(parts[2:]))
	sid[0] = byte(revision)
	sid[1] = byte(len(parts) - 2)
	sid[2] = byte((authority >> 40) & 0xff)
	sid[3] = byte((authority >> 32) & 0xff)
	sid[4] = byte((authority >> 24) & 0xff)
	sid[5] = byte((authority >> 16) & 0xff)
	sid[6] = byte((authority >> 8) & 0xff)
	sid[7] = byte(authority & 0xff)
	offset := 8
	for _, part := range parts[2:] {
		sub, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return nil, err
		}
		sid[offset] = byte(sub)
		sid[offset+1] = byte(sub >> 8)
		sid[offset+2] = byte(sub >> 16)
		sid[offset+3] = byte(sub >> 24)
		offset += 4
	}
	return sid, nil
}

func encodeBinaryLDAPFilter(data []byte) string {
	var b strings.Builder
	for _, v := range data {
		b.WriteString(fmt.Sprintf("\\%02x", v))
	}
	return b.String()
}
