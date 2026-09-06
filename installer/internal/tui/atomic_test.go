package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gentleman-Programming/Gentleman.Dots/installer/internal/system"
)

func atomicSystemInfo() *system.SystemInfo {
	return &system.SystemInfo{
		OS:       system.OSFedora, // Fedora Atomic derivatives report OSFedora
		IsAtomic: true,
		HasBrew:  false,
	}
}

func TestSetupInstallStepsAtomic(t *testing.T) {
	m := NewModel()
	m.SystemInfo = atomicSystemInfo()
	m.Choices = UserChoices{
		OS:        "linux",
		Shell:     "zsh",
		Terminal:  "kitty",
		WindowMgr: "tmux",
	}

	m.SetupInstallSteps()

	var depsStep, terminalStep, homebrewStep *InstallStep
	for i := range m.Steps {
		switch m.Steps[i].ID {
		case "deps":
			depsStep = &m.Steps[i]
		case "terminal":
			terminalStep = &m.Steps[i]
		case "homebrew":
			homebrewStep = &m.Steps[i]
		}
	}

	if depsStep == nil {
		t.Fatal("expected a deps step on an atomic distro")
	}
	if depsStep.Interactive {
		t.Error("deps step must not be Interactive on an atomic distro (no sudo password needed)")
	}

	if terminalStep == nil {
		t.Fatal("expected a terminal step")
	}
	if terminalStep.Interactive {
		t.Error("terminal step must not be Interactive on an atomic distro (no sudo password needed)")
	}

	if homebrewStep == nil {
		t.Error("expected a homebrew step on an atomic distro without brew installed, even though OS reports Fedora")
	}
}

func TestStepInstallDepsAtomicWithoutBrewRecordsManualStep(t *testing.T) {
	m := &Model{
		SystemInfo: atomicSystemInfo(),
		Choices:    UserChoices{OS: "linux", Shell: "zsh"},
	}

	if err := stepInstallDeps(m); err != nil {
		t.Fatalf("stepInstallDeps should not fail on atomic distro, got: %v", err)
	}

	if len(m.ManualSteps) == 0 {
		t.Fatal("expected a manual step to be recorded when Homebrew is unavailable on an atomic distro")
	}
	found := false
	for _, step := range m.ManualSteps {
		if strings.Contains(step, "rpm-ostree") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a manual step mentioning rpm-ostree, got: %v", m.ManualSteps)
	}
}

func TestInstallPlatformPackagesAtomicNoBrewRecordsManualStep(t *testing.T) {
	m := &Model{SystemInfo: atomicSystemInfo()}

	var logs []string
	result := installPlatformPackages(m, "wm", platformPackages{
		Arch:   "tmux",
		Fedora: "tmux",
		Debian: "tmux",
		Brew:   "tmux",
	}, func(line string) { logs = append(logs, line) })

	if result.Error != nil {
		t.Errorf("atomic package install without brew should not report a hard error, got: %v", result.Error)
	}

	foundRpmOstree := false
	for _, step := range m.ManualSteps {
		if strings.Contains(step, "rpm-ostree install tmux") {
			foundRpmOstree = true
		}
	}
	if !foundRpmOstree {
		t.Errorf("expected a manual rpm-ostree step for tmux, got: %v", m.ManualSteps)
	}
}

func TestStepInstallTerminalAtomicAlreadyInstalledCopiesConfig(t *testing.T) {
	tmpHome := t.TempDir()
	m := &Model{
		SystemInfo: atomicSystemInfo(),
		Choices:    UserChoices{OS: "linux", Terminal: "echo"}, // "echo" is guaranteed on PATH, stands in for "already installed"
	}

	err := stepInstallTerminalAtomic(m, "echo", tmpHome, t.TempDir(), "terminal")
	if err != nil {
		t.Fatalf("expected no error for an already-installed terminal, got: %v", err)
	}
	if len(m.ManualSteps) != 0 {
		t.Errorf("no manual step should be recorded when the terminal is already installed, got: %v", m.ManualSteps)
	}
}

func TestStepInstallTerminalAtomicMissingWithoutBrewOrFlatpakRecordsManualStep(t *testing.T) {
	tmpHome := t.TempDir()
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "GentlemanKitty"), 0755); err != nil {
		t.Fatalf("failed to seed fake repo dir: %v", err)
	}
	m := &Model{
		SystemInfo: atomicSystemInfo(), // HasBrew: false, and HasFlatpak defaults to false
		Choices:    UserChoices{OS: "linux", Terminal: "kitty"},
	}

	err := stepInstallTerminalAtomic(m, "kitty", tmpHome, repoDir, "terminal")
	if err != nil {
		t.Fatalf("expected no hard error, got: %v", err)
	}
	if len(m.ManualSteps) == 0 {
		t.Fatal("expected a manual step to be recorded for kitty (no flathub entry, no brew)")
	}
}

func TestStepSetDefaultShellAtomicSkipsSudoAndRecordsManualStep(t *testing.T) {
	m := &Model{
		SystemInfo: atomicSystemInfo(),
		Choices:    UserChoices{OS: "linux", Shell: "zsh"},
	}

	if err := stepSetDefaultShell(m); err != nil {
		t.Fatalf("stepSetDefaultShell should not fail on atomic distro, got: %v", err)
	}

	found := false
	for _, step := range m.ManualSteps {
		if strings.Contains(step, "chsh") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a manual chsh step, got: %v", m.ManualSteps)
	}
}
