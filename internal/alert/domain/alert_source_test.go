package domain

import "testing"

func TestAlertSourceType_IsValid(t *testing.T) {
	valid := []AlertSourceType{
		SourcePrometheusAlertmanager, SourceHuaweiCES, SourceSigNoz, SourceZabbix, SourceCustomWebhook,
	}
	for _, v := range valid {
		if !v.IsValid() {
			t.Fatalf("expected %q to be valid", v)
		}
	}
	if AlertSourceType("foo").IsValid() {
		t.Fatal("expected foo to be invalid")
	}
}
