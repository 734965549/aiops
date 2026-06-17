package domain

import "testing"

func TestTransitionStatusContract(t *testing.T) {
	tests := []struct {
		from   AlertStatus
		action StatusAction
		want   AlertStatus
		ok     bool
	}{
		{StatusNew, ActionAcknowledge, StatusAcknowledged, true},
		{StatusSilenced, ActionAcknowledge, "", false},
		{StatusAcknowledged, ActionStartProcessing, StatusProcessing, true},
		{StatusNew, ActionStartProcessing, "", false},
		{StatusProcessing, ActionRecover, StatusRecovered, true},
		{StatusNew, ActionRecover, "", false},
		{StatusNew, ActionExternalRecover, StatusRecovered, true},
		{StatusAcknowledged, ActionExternalRecover, StatusRecovered, true},
		{StatusProcessing, ActionExternalRecover, StatusRecovered, true},
		{StatusSilenced, ActionExternalRecover, StatusRecovered, true},
		{StatusRecovered, ActionExternalRecover, "", false},
		{StatusClosed, ActionExternalRecover, "", false},
		{StatusRecovered, ActionClose, StatusClosed, true},
		{StatusNew, ActionClose, StatusClosed, true},
		{StatusAcknowledged, ActionClose, StatusClosed, true},
		{StatusProcessing, ActionClose, StatusClosed, true},
		{StatusSilenced, ActionClose, "", false},
		{StatusNew, ActionSilence, StatusSilenced, true},
		{StatusAcknowledged, ActionSilence, StatusSilenced, true},
		{StatusProcessing, ActionSilence, StatusSilenced, true},
		{StatusRecovered, ActionSilence, "", false},
		{StatusSilenced, ActionUnsilence, StatusNew, true},
		{StatusClosed, ActionUnsilence, "", false},
	}
	for _, tc := range tests {
		got, ok := TransitionStatus(tc.from, tc.action)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("TransitionStatus(%s, %s) = (%s, %v), want (%s, %v)",
				tc.from, tc.action, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCanAssignAndComment(t *testing.T) {
	if !CanAssign(StatusNew) || CanAssign(StatusClosed) {
		t.Fatal("assign rules mismatch")
	}
	if !CanComment(StatusProcessing) || CanComment(StatusClosed) {
		t.Fatal("comment rules mismatch")
	}
	if !CanRequestAIAnalysis(StatusProcessing) || CanRequestAIAnalysis(StatusClosed) {
		t.Fatal("ai analysis rules mismatch")
	}
}
