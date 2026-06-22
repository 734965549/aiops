package domain

import (
	"errors"
	"testing"
)

func TestInspectionRun_StateMachine(t *testing.T) {
	run := &InspectionRun{Status: RunStatusPending}
	if err := run.TransitionTo(RunStatusRunning); err != nil {
		t.Fatal(err)
	}
	if run.StartedAt == nil {
		t.Fatal("expected started_at")
	}
	running := &InspectionRun{Status: RunStatusRunning}
	if err := running.TransitionTo(RunStatusSuccess); err != nil {
		t.Fatal(err)
	}
	if running.FinishedAt == nil {
		t.Fatal("expected finished_at")
	}
	done := &InspectionRun{Status: RunStatusSuccess}
	if err := done.TransitionTo(RunStatusRunning); err == nil {
		t.Fatal("expected invalid transition from success")
	}
}

func TestInspectionPolicy_Validate(t *testing.T) {
	p := &InspectionPolicy{Name: "test", Checks: []string{"metrics.cpu"}, Scope: PolicyScope{AccountID: "acc-1"}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if (&InspectionPolicy{Name: "x"}).Validate() == nil {
		t.Fatal("expected scope error")
	}
	invalid := &InspectionPolicy{Name: "test", Checks: []string{"metrics.unknown"}, Scope: PolicyScope{AccountID: "acc-1"}}
	if err := invalid.Validate(); !errors.Is(err, ErrUnsupportedCheck) {
		t.Fatalf("expected unsupported check, got %v", err)
	}
}
