package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/alert/domain"
)

func TestSourceService_CreateDefaultsEnabledTrue(t *testing.T) {
	svc := NewSourceService(&fakeSourceRepo{byID: map[string]*domain.AlertSource{}}, NoopAuditRecorder{})
	out, err := svc.Create(context.Background(), Actor{}, CreateSourceInput{
		ID:     "prod-am",
		Name:   "Prod AM",
		Secret: "webhook-secret",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !out.Enabled {
		t.Fatal("expected enabled=true when omitted")
	}
}

func TestSourceService_CreateExplicitEnabledFalse(t *testing.T) {
	enabled := false
	svc := NewSourceService(&fakeSourceRepo{byID: map[string]*domain.AlertSource{}}, NoopAuditRecorder{})
	out, err := svc.Create(context.Background(), Actor{}, CreateSourceInput{
		ID:      "staging-am",
		Name:    "Staging AM",
		Secret:  "webhook-secret",
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Enabled {
		t.Fatal("expected enabled=false when explicitly set")
	}
}

func TestSourceService_UpdatePreservesEnabledWhenOmitted(t *testing.T) {
	repo := &fakeSourceRepo{byID: map[string]*domain.AlertSource{}}
	svc := NewSourceService(repo, NoopAuditRecorder{})
	ctx := context.Background()

	enabled := true
	if _, err := svc.Create(ctx, Actor{}, CreateSourceInput{
		ID: "prod-am", Name: "Prod AM", Secret: "webhook-secret", Enabled: &enabled,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	out, err := svc.Update(ctx, "prod-am", Actor{}, UpdateSourceInput{Name: strPtr("Prod AM v2")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !out.Enabled {
		t.Fatal("expected enabled preserved on partial update")
	}
	if out.Name != "Prod AM v2" {
		t.Fatalf("expected name updated, got %q", out.Name)
	}
}

func TestSourceService_UpdateExplicitEnabledFalse(t *testing.T) {
	repo := &fakeSourceRepo{byID: map[string]*domain.AlertSource{}}
	svc := NewSourceService(repo, NoopAuditRecorder{})
	ctx := context.Background()

	if _, err := svc.Create(ctx, Actor{}, CreateSourceInput{
		ID: "prod-am", Name: "Prod AM", Secret: "webhook-secret",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	disabled := false
	out, err := svc.Update(ctx, "prod-am", Actor{}, UpdateSourceInput{Enabled: &disabled})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.Enabled {
		t.Fatal("expected enabled=false when explicitly set")
	}
}

func strPtr(s string) *string { return &s }

func TestSourceService_CreateInvalidType(t *testing.T) {
	svc := NewSourceService(&fakeSourceRepo{byID: map[string]*domain.AlertSource{}}, NoopAuditRecorder{})
	_, err := svc.Create(context.Background(), Actor{}, CreateSourceInput{
		ID:     "prod-am",
		Name:   "Prod AM",
		Secret: "webhook-secret",
		Type:   "foo",
	})
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestSourceService_UpdateClearsOptionalFields(t *testing.T) {
	repo := &fakeSourceRepo{byID: map[string]*domain.AlertSource{}}
	svc := NewSourceService(repo, NoopAuditRecorder{})
	ctx := context.Background()

	if _, err := svc.Create(ctx, Actor{}, CreateSourceInput{
		ID: "prod-am", Name: "Prod AM", Secret: "webhook-secret",
		Environment: "prod", BusinessLine: "payment", Description: "note",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	empty := ""
	out, err := svc.Update(ctx, "prod-am", Actor{}, UpdateSourceInput{
		Environment:  &empty,
		BusinessLine: &empty,
		Description:  &empty,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.Environment != "" || out.BusinessLine != "" || out.Description != "" {
		t.Fatalf("expected optional fields cleared, got env=%q line=%q desc=%q",
			out.Environment, out.BusinessLine, out.Description)
	}
}

func TestSourceService_UpdateInvalidType(t *testing.T) {
	repo := &fakeSourceRepo{byID: map[string]*domain.AlertSource{}}
	svc := NewSourceService(repo, NoopAuditRecorder{})
	ctx := context.Background()

	if _, err := svc.Create(ctx, Actor{}, CreateSourceInput{
		ID: "prod-am", Name: "Prod AM", Secret: "webhook-secret",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	bad := "foo"
	_, err := svc.Update(ctx, "prod-am", Actor{}, UpdateSourceInput{Type: &bad})
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}
