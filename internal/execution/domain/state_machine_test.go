package domain

import "testing"

func TestCanTransitionTo_Contract(t *testing.T) {
	cases := []struct {
		from, to TaskStatus
		want     bool
	}{
		{StatusPendingConfirm, StatusPendingExecute, true},
		{StatusPendingConfirm, StatusCancelled, true},
		{StatusPendingExecute, StatusRunning, true},
		{StatusRunning, StatusSuccess, true},
		{StatusRunning, StatusFailed, true},
		{StatusPendingConfirm, StatusRunning, false},
		{StatusPendingExecute, StatusSuccess, false},
		{StatusSuccess, StatusRunning, false},
		{StatusCancelled, StatusPendingExecute, false},
	}
	for _, tc := range cases {
		got := CanTransitionTo(tc.from, tc.to)
		if got != tc.want {
			t.Fatalf("CanTransitionTo(%s, %s) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestTransitionForAction(t *testing.T) {
	cases := []struct {
		action   TaskAction
		wantFrom TaskStatus
		wantTo   TaskStatus
		wantErr  bool
	}{
		{ActionConfirm, StatusPendingConfirm, StatusPendingExecute, false},
		{ActionReject, StatusPendingConfirm, StatusCancelled, false},
		{ActionExecute, StatusPendingExecute, StatusRunning, false},
		{TaskAction("unknown"), "", "", true},
	}
	for _, tc := range cases {
		from, to, err := TransitionForAction(tc.action)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("TransitionForAction(%s) expected error", tc.action)
			}
			continue
		}
		if err != nil {
			t.Fatalf("TransitionForAction(%s): %v", tc.action, err)
		}
		if from != tc.wantFrom || to != tc.wantTo {
			t.Fatalf("TransitionForAction(%s) = (%s, %s), want (%s, %s)",
				tc.action, from, to, tc.wantFrom, tc.wantTo)
		}
	}
}

func TestTransitionHelpers(t *testing.T) {
	to, err := TransitionConfirm(StatusPendingConfirm)
	if err != nil || to != StatusPendingExecute {
		t.Fatalf("TransitionConfirm: got (%s, %v)", to, err)
	}
	if _, err := TransitionConfirm(StatusPendingExecute); err == nil {
		t.Fatal("TransitionConfirm from pending_execute should fail")
	}

	to, err = TransitionReject(StatusPendingConfirm)
	if err != nil || to != StatusCancelled {
		t.Fatalf("TransitionReject: got (%s, %v)", to, err)
	}

	to, err = TransitionExecute(StatusPendingExecute)
	if err != nil || to != StatusRunning {
		t.Fatalf("TransitionExecute: got (%s, %v)", to, err)
	}
	if _, err := TransitionExecute(StatusPendingConfirm); err == nil {
		t.Fatal("TransitionExecute from pending_confirm should fail")
	}
}
