package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
)

type capturingAssetAudit struct {
	rows []AuditRecord
	err  error
}

func (c *capturingAssetAudit) Record(_ context.Context, rec AuditRecord) error {
	if c.err != nil {
		return c.err
	}
	c.rows = append(c.rows, rec)
	return nil
}

func TestAssetService_CreateApplicationWritesAudit(t *testing.T) {
	audit := &capturingAssetAudit{}
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	svc := NewAssetService(apps, &fakeResRepo{}, nil, nil, audit)
	out, err := svc.CreateApplication(context.Background(), Actor{UserID: "user-1"}, CreateApplicationInput{
		Name: "payment-service", Environment: "prod",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(audit.rows))
	}
	row := audit.rows[0]
	if row.ResourceType != "application" || row.ResourceID != out.ID || row.UserID != "user-1" {
		t.Fatalf("unexpected audit: %+v", row)
	}
	if row.Payload["result"] != "success" {
		t.Fatalf("expected success result, got %+v", row.Payload)
	}
}

func TestAssetService_CreateApplicationAuditFailureDoesNotBlock(t *testing.T) {
	audit := &capturingAssetAudit{err: errors.New("audit down")}
	svc := NewAssetService(&fakeAppRepo{apps: map[string]domain.Application{}}, &fakeResRepo{}, nil, nil, audit)
	out, err := svc.CreateApplication(context.Background(), Actor{UserID: "user-1"}, CreateApplicationInput{Name: "svc"})
	if err != nil {
		t.Fatalf("create should succeed despite audit failure: %v", err)
	}
	if out == nil || out.Name != "svc" {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestAssetService_CreateResourceWritesAudit(t *testing.T) {
	audit := &capturingAssetAudit{}
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}}
	resources := &fakeResRepo{}
	svc := NewAssetService(apps, resources, nil, nil, audit)
	out, err := svc.CreateResource(context.Background(), Actor{UserID: "user-2"}, CreateResourceInput{
		ApplicationID: "app-1", Name: "pod-1", ResourceType: "pod",
	})
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if len(audit.rows) != 1 || audit.rows[0].ResourceType != "resource" || audit.rows[0].ResourceID != out.ID {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
}
