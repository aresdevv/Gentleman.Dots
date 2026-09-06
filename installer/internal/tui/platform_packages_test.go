package tui

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Gentleman-Programming/Gentleman.Dots/installer/internal/system"
)

type packageCommandCall struct {
	runner  string
	command string
}

func withPackageCommandMocks(t *testing.T, sudoErr error) *[]packageCommandCall {
	t.Helper()

	originalPkg := runPkgInstallWithLogs
	originalSudo := runSudoWithLogs
	originalBrew := runBrewWithLogs

	calls := []packageCommandCall{}

	runPkgInstallWithLogs = func(packages string, opts *system.ExecOptions, onLog func(string)) *system.ExecResult {
		calls = append(calls, packageCommandCall{runner: "pkg", command: packages})
		return &system.ExecResult{Command: packages}
	}
	runSudoWithLogs = func(command string, opts *system.ExecOptions, onLog system.LogCallback) *system.ExecResult {
		calls = append(calls, packageCommandCall{runner: "sudo", command: command})
		return &system.ExecResult{Command: command, Error: sudoErr}
	}
	runBrewWithLogs = func(args string, opts *system.ExecOptions, onLog system.LogCallback) *system.ExecResult {
		calls = append(calls, packageCommandCall{runner: "brew", command: args})
		return &system.ExecResult{Command: args}
	}

	t.Cleanup(func() {
		runPkgInstallWithLogs = originalPkg
		runSudoWithLogs = originalSudo
		runBrewWithLogs = originalBrew
	})

	return &calls
}

// withArchAURMocks additionally mocks the AUR helper detection and
// invocation seams used by installArchAURPackages.
func withArchAURMocks(t *testing.T, helper string, aurErr error) *[]packageCommandCall {
	t.Helper()

	calls := withPackageCommandMocks(t, nil)

	originalDetect := detectAURHelper
	originalAUR := runAURWithLogs

	detectAURHelper = func() string { return helper }
	runAURWithLogs = func(command string, opts *system.ExecOptions, onLog system.LogCallback) *system.ExecResult {
		*calls = append(*calls, packageCommandCall{runner: "aur", command: command})
		return &system.ExecResult{Command: command, Error: aurErr}
	}

	t.Cleanup(func() {
		detectAURHelper = originalDetect
		runAURWithLogs = originalAUR
	})

	return calls
}

func TestInstallPlatformPackagesFedoraFallsBackToBrewWhenNativeFails(t *testing.T) {
	calls := withPackageCommandMocks(t, errors.New("dnf failed"))

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSFedora, HasBrew: true}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Brew:   "fish carapace zoxide atuin starship",
		Fedora: "fish carapace zoxide atuin starship",
	}, nil)

	if result.Error != nil {
		t.Fatalf("expected brew fallback to succeed, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "sudo", command: "dnf install -y fish carapace zoxide atuin starship"},
		{runner: "brew", command: "install fish carapace zoxide atuin starship"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v", *calls, expected)
	}
}

func TestInstallPlatformPackagesDebianWithBrewUsesBrewDirectly(t *testing.T) {
	calls := withPackageCommandMocks(t, nil)

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSDebian, HasBrew: true}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Brew:   "fish carapace zoxide atuin starship",
		Debian: "fish zoxide starship",
	}, nil)

	if result.Error != nil {
		t.Fatalf("expected brew install to succeed, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "brew", command: "install fish carapace zoxide atuin starship"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v", *calls, expected)
	}
}

func TestInstallPlatformPackagesDebianWithoutBrewUsesApt(t *testing.T) {
	calls := withPackageCommandMocks(t, nil)

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSDebian, HasBrew: false}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Brew:   "fish carapace zoxide atuin starship",
		Debian: "fish zoxide starship",
	}, nil)

	if result.Error != nil {
		t.Fatalf("expected apt install to succeed, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "sudo", command: "apt-get install -y fish zoxide starship"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v", *calls, expected)
	}
}

func TestInstallPlatformPackagesArchFallsBackToBrewWhenNativeFails(t *testing.T) {
	calls := withPackageCommandMocks(t, errors.New("pacman failed"))

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSArch, HasBrew: true}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Brew: "fish carapace zoxide atuin starship",
		Arch: "fish carapace zoxide atuin starship",
	}, nil)

	if result.Error != nil {
		t.Fatalf("expected brew fallback to succeed, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "sudo", command: "pacman -S --needed --noconfirm fish carapace zoxide atuin starship"},
		{runner: "brew", command: "install fish carapace zoxide atuin starship"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v", *calls, expected)
	}
}

// TestArchAURPackagesInstalledViaDetectedHelper proves AUR-only packages are
// installed through the detected helper (yay/paru), never handed to pacman.
func TestArchAURPackagesInstalledViaDetectedHelper(t *testing.T) {
	calls := withArchAURMocks(t, "yay", nil)

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSArch, HasBrew: false}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Arch:    "fish zoxide atuin starship",
		ArchAUR: "carapace-bin",
	}, nil)

	if result.Error != nil {
		t.Fatalf("expected install to succeed, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "sudo", command: "pacman -S --needed --noconfirm fish zoxide atuin starship"},
		{runner: "aur", command: "yay -S --needed --noconfirm carapace-bin"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v", *calls, expected)
	}

	for _, c := range *calls {
		if c.runner == "sudo" && strings.Contains(c.command, "carapace") {
			t.Fatalf("AUR-only package leaked into pacman command: %q", c.command)
		}
	}
}

// TestArchAURPackagesPreferParuWhenYayMissing proves paru is used when yay
// is not installed but paru is.
func TestArchAURPackagesPreferParuWhenYayMissing(t *testing.T) {
	calls := withArchAURMocks(t, "paru", nil)

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSArch, HasBrew: false}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Arch:    "zsh zoxide atuin",
		ArchAUR: "carapace-bin zsh-theme-powerlevel10k",
	}, nil)

	if result.Error != nil {
		t.Fatalf("expected install to succeed, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "sudo", command: "pacman -S --needed --noconfirm zsh zoxide atuin"},
		{runner: "aur", command: "paru -S --needed --noconfirm carapace-bin zsh-theme-powerlevel10k"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v", *calls, expected)
	}
}

// TestArchAURPackagesSkippedWithoutHelperDoesNotFail proves the installer
// does not hard-fail (and does not require Homebrew) when there is no AUR
// helper available on Arch: it must skip the AUR-only packages with a
// non-fatal warning instead of aborting the step.
func TestArchAURPackagesSkippedWithoutHelperDoesNotFail(t *testing.T) {
	calls := withArchAURMocks(t, "", nil)

	var warnings []string
	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSArch, HasBrew: false}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Arch:    "fish zoxide atuin starship",
		ArchAUR: "carapace-bin",
	}, func(line string) {
		warnings = append(warnings, line)
	})

	if result.Error != nil {
		t.Fatalf("expected non-fatal skip, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "sudo", command: "pacman -S --needed --noconfirm fish zoxide atuin starship"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v (no AUR helper should be invoked)", *calls, expected)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "carapace-bin") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning mentioning the skipped AUR package, got logs: %v", warnings)
	}
}

// TestArchOfficialPackagesFailStillReturnsErrorWithoutAURAttempt proves that
// when the official pacman install itself fails (and no brew fallback
// applies), AUR packages are not attempted and the error propagates.
func TestArchOfficialPackagesFailStillReturnsErrorWithoutAURAttempt(t *testing.T) {
	calls := withArchAURMocks(t, "yay", nil)
	runSudoWithLogs = func(command string, opts *system.ExecOptions, onLog system.LogCallback) *system.ExecResult {
		*calls = append(*calls, packageCommandCall{runner: "sudo", command: command})
		return &system.ExecResult{Command: command, Error: errors.New("pacman failed")}
	}

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSArch, HasBrew: false}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Arch:    "fish zoxide atuin starship",
		ArchAUR: "carapace-bin",
	}, nil)

	if result.Error == nil {
		t.Fatalf("expected pacman failure to propagate")
	}

	for _, c := range *calls {
		if c.runner == "aur" {
			t.Fatalf("AUR helper must not run when the official package install failed: %#v", *calls)
		}
	}
}

// TestArchPackageListsExcludeAURNames statically guards the real production
// package literals in installer.go: AUR-only packages (carapace,
// zsh-theme-powerlevel10k) must never appear in an Arch (pacman) field.
func TestArchPackageListsExcludeAURNames(t *testing.T) {
	src, err := os.ReadFile("installer.go")
	if err != nil {
		t.Fatalf("failed to read installer.go: %v", err)
	}

	forbidden := []string{"carapace", "zsh-theme-powerlevel10k"}

	for i, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Arch:") {
			continue
		}
		for _, name := range forbidden {
			if strings.Contains(trimmed, name) {
				t.Errorf("installer.go:%d: pacman-only Arch package list must not contain AUR-only package %q (use ArchAUR): %s", i+1, name, trimmed)
			}
		}
	}
}
