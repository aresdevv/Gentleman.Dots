package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Gentleman-Programming/Gentleman.Dots/installer/internal/system"
)

// TestExecuteStepDryRunSkipsMutation verifies that executeStep short-circuits
// before running any step body when Model.DryRun is set, regardless of which
// step is requested (package installs, clone, backup, shell patch, ...).
func TestExecuteStepDryRunSkipsMutation(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(origWd)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	m := &Model{
		SystemInfo: &system.SystemInfo{OS: system.OSLinux},
		Choices: UserChoices{
			OS:           "linux",
			Shell:        "zsh",
			Terminal:     "kitty",
			WindowMgr:    "tmux",
			CreateBackup: true,
		},
		ExistingConfigs: []string{"zsh: " + filepath.Join(tmpDir, ".zshrc")},
		DryRun:          true,
	}

	for _, stepID := range []string{"backup", "clone", "deps", "terminal", "shell", "wm", "setshell", "cleanup"} {
		if err := executeStep(stepID, m); err != nil {
			t.Errorf("executeStep(%q) under dry-run returned error: %v", stepID, err)
		}
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "Gentleman.Dots")); !os.IsNotExist(err) {
		t.Error("dry-run must not clone the repository")
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read tmp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("dry-run must not write/create anything in the working directory, found: %v", entries)
	}
}

// TestRunInteractiveStepDryRunSkipsScript verifies the TUI's interactive
// (sudo/chsh) code path never builds or executes a temp script under dry-run.
func TestRunInteractiveStepDryRunSkipsScript(t *testing.T) {
	m := &Model{
		SystemInfo: &system.SystemInfo{OS: system.OSLinux},
		Choices:    UserChoices{OS: "linux", Terminal: "kitty"},
		DryRun:     true,
	}

	cmd := runInteractiveStep("deps", m)
	if cmd == nil {
		t.Fatal("expected a non-nil tea.Cmd")
	}

	msg := cmd()
	finished, ok := msg.(execFinishedMsg)
	if !ok {
		t.Fatalf("expected execFinishedMsg under dry-run, got %T (%v)", msg, msg)
	}
	if finished.err != nil {
		t.Errorf("expected no error under dry-run, got: %v", finished.err)
	}
}

// TestRunNonInteractiveDryRunLeavesHomeUntouched exercises the full
// non-interactive entrypoint end-to-end and asserts it performs zero
// filesystem mutation when DryRun is requested.
func TestRunNonInteractiveDryRunLeavesHomeUntouched(t *testing.T) {
	tmpHome := t.TempDir()
	tmpWorkDir := t.TempDir()

	origHome := os.Getenv("HOME")
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Chdir(origWd)
		system.SetDryRun(false)
	}()

	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	if err := os.Chdir(tmpWorkDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	choices := UserChoices{
		Shell:        "zsh",
		Terminal:     "none",
		WindowMgr:    "none",
		CreateBackup: true,
		DryRun:       true,
	}

	if err := RunNonInteractive(choices); err != nil {
		t.Fatalf("RunNonInteractive under dry-run should not fail: %v", err)
	}

	if !system.IsDryRun() {
		t.Error("expected the process-wide dry-run flag to be set by RunNonInteractive")
	}

	if _, statErr := os.Stat(filepath.Join(tmpWorkDir, "Gentleman.Dots")); !os.IsNotExist(statErr) {
		t.Error("dry-run must not clone the repository")
	}

	homeEntries, err := os.ReadDir(tmpHome)
	if err != nil {
		t.Fatalf("failed to read tmp home: %v", err)
	}
	if len(homeEntries) != 0 {
		t.Errorf("dry-run must leave HOME untouched, found entries: %v", homeEntries)
	}
}
