package system

import "testing"

func TestDryRunFlag(t *testing.T) {
	// Restore whatever state other tests may expect.
	defer SetDryRun(false)

	if IsDryRun() {
		t.Fatal("expected dry-run to default to false")
	}

	SetDryRun(true)
	if !IsDryRun() {
		t.Error("expected IsDryRun to report true after SetDryRun(true)")
	}

	SetDryRun(false)
	if IsDryRun() {
		t.Error("expected IsDryRun to report false after SetDryRun(false)")
	}
}
