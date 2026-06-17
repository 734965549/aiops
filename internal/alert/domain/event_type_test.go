package domain

import "testing"

func TestAlertEventTypeIsValid(t *testing.T) {
	cases := []AlertEventType{
		EventTriggered, EventUpdated, EventRecovered, EventAcknowledged, EventAssigned,
		EventProcessingStarted, EventClosed, EventSilenced, EventUnsilenced, EventCommented,
		EventAIAnalysisRequested, EventExecutionCreated, EventExecutionStarted, EventExecutionFinished,
	}
	for _, c := range cases {
		if !c.IsValid() {
			t.Fatalf("%s should be valid", c)
		}
	}
	if (AlertEventType("unknown")).IsValid() {
		t.Fatal("unknown event type should be invalid")
	}
}
