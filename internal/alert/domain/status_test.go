package domain

import "testing"

func TestAlertStatusIsTerminalAndActive(t *testing.T) {
	if !StatusClosed.IsTerminal() {
		t.Fatal("closed should be terminal")
	}
	if StatusNew.IsTerminal() {
		t.Fatal("new should not be terminal")
	}
	if !StatusSilenced.IsActive() {
		t.Fatal("silenced should be active")
	}
	if StatusClosed.IsActive() {
		t.Fatal("closed should not be active")
	}
}

func TestAlertStatusDisplayLabel(t *testing.T) {
	if StatusAcknowledged.DisplayLabel() != "已认领" {
		t.Fatalf("unexpected label: %s", StatusAcknowledged.DisplayLabel())
	}
}

func TestAlertStatusIsValid(t *testing.T) {
	if !StatusProcessing.IsValid() {
		t.Fatal("processing should be valid")
	}
	if (AlertStatus("bogus")).IsValid() {
		t.Fatal("bogus status should be invalid")
	}
}
