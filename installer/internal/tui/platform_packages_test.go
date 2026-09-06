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

// TestFedoraExtraPackagesInstalledViaBrewWhenAvailable proves packages that
// are not in Fedora's official repos (starship, carapace, lazygit, zellij:
// none exist there as of Fedora 44) are installed via Homebrew rather than
// being handed to dnf, and never poison the dnf transaction.
func TestFedoraExtraPackagesInstalledViaBrewWhenAvailable(t *testing.T) {
	calls := withPackageCommandMocks(t, nil)

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSFedora, HasBrew: true}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Fedora:      "fish zoxide atuin",
		FedoraExtra: "carapace starship",
	}, nil)

	if result.Error != nil {
		t.Fatalf("expected install to succeed, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "sudo", command: "dnf install -y fish zoxide atuin"},
		{runner: "brew", command: "install carapace starship"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v", *calls, expected)
	}

	for _, c := range *calls {
		if c.runner == "sudo" && (strings.Contains(c.command, "starship") || strings.Contains(c.command, "carapace")) {
			t.Fatalf("package not in Fedora's repos leaked into dnf command: %q", c.command)
		}
	}
}

// TestFedoraExtraPackagesSkippedWithoutBrewDoesNotFail proves the installer
// does not hard-fail when a Fedora user has no Homebrew installed: missing
// packages are skipped with a non-fatal warning instead of aborting.
func TestFedoraExtraPackagesSkippedWithoutBrewDoesNotFail(t *testing.T) {
	calls := withPackageCommandMocks(t, nil)

	var warnings []string
	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSFedora, HasBrew: false}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Fedora:      "fish zoxide atuin",
		FedoraExtra: "carapace starship",
	}, func(line string) {
		warnings = append(warnings, line)
	})

	if result.Error != nil {
		t.Fatalf("expected non-fatal skip, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "sudo", command: "dnf install -y fish zoxide atuin"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v (brew must not be invoked)", *calls, expected)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "carapace") && strings.Contains(w, "starship") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning mentioning the skipped packages, got logs: %v", warnings)
	}
}

// TestFedoraOfficialFailureSkipsExtraPackages proves that when the official
// dnf install itself fails outright (and no brew fallback applies), the
// FedoraExtra packages are never attempted and the error propagates.
func TestFedoraOfficialFailureSkipsExtraPackages(t *testing.T) {
	calls := withPackageCommandMocks(t, errors.New("dnf failed"))

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSFedora, HasBrew: false}}
	result := installPlatformPackages(m, "shell", platformPackages{
		Fedora:      "fish zoxide atuin",
		FedoraExtra: "carapace starship",
	}, nil)

	if result.Error == nil {
		t.Fatalf("expected dnf failure to propagate")
	}

	for _, c := range *calls {
		if c.runner == "brew" {
			t.Fatalf("brew must not run when the official dnf install failed: %#v", *calls)
		}
	}
}

// TestFedoraOnlyFedoraExtraStillInstalls proves a step whose Fedora field is
// entirely empty (all its packages are FedoraExtra-only, e.g. zellij) still
// attempts the extra-package install instead of silently doing nothing.
func TestFedoraOnlyFedoraExtraStillInstalls(t *testing.T) {
	calls := withPackageCommandMocks(t, nil)

	m := &Model{SystemInfo: &system.SystemInfo{OS: system.OSFedora, HasBrew: true}}
	result := installPlatformPackages(m, "wm", platformPackages{
		FedoraExtra: "zellij",
	}, nil)

	if result.Error != nil {
		t.Fatalf("expected install to succeed, got error: %v", result.Error)
	}

	expected := []packageCommandCall{
		{runner: "brew", command: "install zellij"},
	}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("calls = %#v, want %#v", *calls, expected)
	}
}

// TestFedoraPackageListsExcludeKnownInvalidNames statically guards the real
// production package literals in installer.go against packages verified to
// not exist in Fedora's official repos as of Fedora 44 (starship, carapace,
// lazygit, zellij all returned zero results on
// packages.fedoraproject.org), a bare "npm" (Fedora ships it as the
// "nodejs-npm" subpackage), and the DNF4-only "@development-tools" group
// shorthand (DNF5, Fedora's default since Fedora 41, does not recognize
// it - "dnf group install" must be used instead).
func TestFedoraPackageListsExcludeKnownInvalidNames(t *testing.T) {
	src, err := os.ReadFile("installer.go")
	if err != nil {
		t.Fatalf("failed to read installer.go: %v", err)
	}
	text := string(src)

	interactiveSrc, err := os.ReadFile("interactive.go")
	if err != nil {
		t.Fatalf("failed to read interactive.go: %v", err)
	}

	for _, f := range []struct {
		name string
		text string
	}{
		{"installer.go", text},
		{"interactive.go", string(interactiveSrc)},
	} {
		if strings.Contains(f.text, "install -y @development-tools") {
			t.Errorf("%s must not run \"dnf install -y @development-tools\"; DNF5 (Fedora's default dnf) does not recognize that DNF4-only group shorthand - use \"dnf group install\" instead", f.name)
		}
		for i, line := range strings.Split(f.text, "\n") {
			if strings.Contains(line, "dnf install") && strings.Contains(line, "wget") && !strings.Contains(line, "wget2") {
				t.Errorf("%s:%d: wget was retired from Fedora's repos in favor of wget2 (wget2-wget provides the compat binary): %s", f.name, i+1, strings.TrimSpace(line))
			}
		}
	}

	forbidden := []string{"starship", "carapace", "lazygit", "zellij"}
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Fedora:") {
			continue
		}
		if strings.Contains(trimmed, "npm") && !strings.Contains(trimmed, "nodejs-npm") {
			t.Errorf("installer.go:%d: Fedora has no standalone \"npm\" package, use \"nodejs-npm\": %s", i+1, trimmed)
		}
		for _, name := range forbidden {
			if strings.Contains(trimmed, name) {
				t.Errorf("installer.go:%d: %q is not available in Fedora's official repos, use FedoraExtra instead: %s", i+1, name, trimmed)
			}
		}
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
