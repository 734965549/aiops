package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/integration/domain"
)

func TestHuaweiCloudCheckerFieldValidationAKSK(t *testing.T) {
	checker := NewHuaweiCloudChecker()
	account := domain.IntegrationAccount{
		AccountID: "acc-1",
		Provider:  domain.ProviderHuaweiCloud,
		AuthType:  domain.AuthAKSK,
		Regions:   []string{"cn-north-4"},
		ProjectID: "proj-1",
	}
	check, err := checker.CheckConnectivity(context.Background(), account, domain.CredentialMaterial{
		"access_key": "AKTEST",
		"secret_key": "SKTEST",
	})
	if err != nil {
		t.Fatalf("CheckConnectivity: %v", err)
	}
	if check.Status != domain.ConnectivityOK {
		t.Fatalf("expected ok, got %q", check.Status)
	}
}

func TestHuaweiCloudCheckerRequiresCredentialFields(t *testing.T) {
	checker := NewHuaweiCloudChecker()
	_, err := checker.CheckConnectivity(context.Background(), domain.IntegrationAccount{
		AccountID: "acc-2",
		Provider:  domain.ProviderHuaweiCloud,
		AuthType:  domain.AuthAKSK,
		Regions:   []string{"cn-north-4"},
	}, domain.CredentialMaterial{"access_key": "AKONLY"})
	if !errors.Is(err, domain.ErrCredentialRequired) {
		t.Fatalf("expected ErrCredentialRequired, got %v", err)
	}
}

func TestHuaweiCloudCheckerRequiresRegionsForRealAuth(t *testing.T) {
	checker := NewHuaweiCloudChecker()
	_, err := checker.CheckConnectivity(context.Background(), domain.IntegrationAccount{
		AccountID: "acc-3",
		Provider:  domain.ProviderHuaweiCloud,
		AuthType:  domain.AuthAKSK,
	}, domain.CredentialMaterial{
		"access_key": "AKTEST",
		"secret_key": "SKTEST",
	})
	if !errors.Is(err, domain.ErrConnectivityFailed) {
		t.Fatalf("expected ErrConnectivityFailed, got %v", err)
	}
}
