package system

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOSTypes(t *testing.T) {
	t.Run("OSFedora should be defined", func(t *testing.T) {
		// Verify OSFedora is a valid OSType
		var osType OSType = OSFedora
		if osType == OSUnknown {
			t.Error("OSFedora should not equal OSUnknown")
		}
	})

	t.Run("all OS types should be distinct", func(t *testing.T) {
		osTypes := []OSType{OSMac, OSLinux, OSArch, OSDebian, OSFedora, OSTermux, OSUnknown}
		seen := make(map[OSType]bool)
		for _, ot := range osTypes {
			if seen[ot] {
				t.Errorf("Duplicate OS type value found: %d", ot)
			}
			seen[ot] = true
		}
	})
}

func TestDetect(t *testing.T) {
	info := Detect()

	t.Run("should detect OS", func(t *testing.T) {
		if info.OS == OSUnknown && runtime.GOOS != "windows" {
			t.Error("OS should not be unknown on unix systems")
		}
	})

	t.Run("should have home directory", func(t *testing.T) {
		if info.HomeDir == "" {
			t.Error("HomeDir should not be empty")
		}
	})

	t.Run("should detect current shell", func(t *testing.T) {
		// Only test if SHELL env is set
		if os.Getenv("SHELL") != "" && info.UserShell == "unknown" {
			t.Error("UserShell should be detected when SHELL env is set")
		}
	})

	t.Run("OSName should match OS type", func(t *testing.T) {
		switch runtime.GOOS {
		case "darwin":
			if info.OSName != "macOS" {
				t.Errorf("Expected OSName to be 'macOS', got '%s'", info.OSName)
			}
		case "linux":
			validNames := []string{"Linux", "Arch Linux", "Debian/Ubuntu", "Fedora/RHEL", "Termux"}
			found := false
			for _, name := range validNames {
				if info.OSName == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Unexpected OSName for Linux: '%s'", info.OSName)
			}
		}
	})
}

func TestCommandExists(t *testing.T) {
	t.Run("should find common commands", func(t *testing.T) {
		// These should exist on any unix system
		commonCmds := []string{"ls", "echo", "cat"}
		for _, cmd := range commonCmds {
			if !CommandExists(cmd) {
				t.Errorf("Command '%s' should exist", cmd)
			}
		}
	})

	t.Run("should not find non-existent commands", func(t *testing.T) {
		if CommandExists("this-command-definitely-does-not-exist-xyz123") {
			t.Error("Should not find non-existent command")
		}
	})
}

func TestGetBrewPrefix(t *testing.T) {
	prefix := GetBrewPrefix()

	t.Run("should return valid prefix based on OS and arch", func(t *testing.T) {
		switch runtime.GOOS {
		case "darwin":
			if runtime.GOARCH == "arm64" {
				if prefix != "/opt/homebrew" {
					t.Errorf("Expected '/opt/homebrew' on macOS ARM64, got '%s'", prefix)
				}
			} else {
				if prefix != "/usr/local" {
					t.Errorf("Expected '/usr/local' on macOS Intel, got '%s'", prefix)
				}
			}
		case "linux":
			if prefix != "/home/linuxbrew/.linuxbrew" {
				t.Errorf("Expected '/home/linuxbrew/.linuxbrew' on Linux, got '%s'", prefix)
			}
		}
	})
}

// TestResolveBrewCommand covers issue #192: a Homebrew install that is
// genuinely on PATH must be preferred over GetBrewPrefix's hardcoded guess,
// since a non-default install location (e.g. a per-user
// "$HOME/.linuxbrew" install) would otherwise never be found.
func TestResolveBrewCommand(t *testing.T) {
	originalPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", originalPath) })

	t.Run("prefers brew resolvable on PATH over the hardcoded prefix", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmpDir, "brew"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatalf("failed to create fake brew: %v", err)
		}
		if err := os.Setenv("PATH", tmpDir); err != nil {
			t.Fatalf("failed to set PATH: %v", err)
		}

		if got := ResolveBrewCommand(); got != "brew" {
			t.Errorf("ResolveBrewCommand() = %q, want %q (bare command, resolved via PATH)", got, "brew")
		}
	})

	t.Run("falls back to the hardcoded prefix when brew is not on PATH", func(t *testing.T) {
		if err := os.Setenv("PATH", t.TempDir()); err != nil {
			t.Fatalf("failed to clear PATH: %v", err)
		}

		want := GetBrewPrefix() + "/bin/brew"
		if got := ResolveBrewCommand(); got != want {
			t.Errorf("ResolveBrewCommand() = %q, want %q", got, want)
		}
	})
}

func TestCheckWSL(t *testing.T) {
	// This test just ensures the function doesn't panic
	t.Run("should not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("checkWSL panicked: %v", r)
			}
		}()
		_ = checkWSL()
	})
}

func TestIsArchLinux(t *testing.T) {
	t.Run("should not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("isArchLinux panicked: %v", r)
			}
		}()
		_ = isArchLinux()
	})
}

func TestIsDebian(t *testing.T) {
	t.Run("should not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("isDebian panicked: %v", r)
			}
		}()
		_ = isDebian()
	})
}

func TestIsFedora(t *testing.T) {
	t.Run("should not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("isFedora panicked: %v", r)
			}
		}()
		_ = isFedora()
	})

	t.Run("should return bool", func(t *testing.T) {
		result := isFedora()
		// Just verify it returns a boolean without panicking
		if result {
			t.Log("Running on Fedora/RHEL system")
		} else {
			t.Log("Not running on Fedora/RHEL system")
		}
	})
}

func TestIsTermux(t *testing.T) {
	t.Run("should not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("isTermux panicked: %v", r)
			}
		}()
		_ = isTermux()
	})

	t.Run("should detect TERMUX_VERSION env", func(t *testing.T) {
		// Save original value
		original := os.Getenv("TERMUX_VERSION")
		defer os.Setenv("TERMUX_VERSION", original)

		// Set Termux env
		os.Setenv("TERMUX_VERSION", "0.118.0")
		if !isTermux() {
			t.Error("Should detect Termux when TERMUX_VERSION is set")
		}

		// Unset
		os.Unsetenv("TERMUX_VERSION")
		// Note: might still be true if PREFIX contains termux
	})

	t.Run("should detect PREFIX with termux path", func(t *testing.T) {
		// Save original values
		originalVersion := os.Getenv("TERMUX_VERSION")
		originalPrefix := os.Getenv("PREFIX")
		defer func() {
			os.Setenv("TERMUX_VERSION", originalVersion)
			os.Setenv("PREFIX", originalPrefix)
		}()

		// Clear TERMUX_VERSION, set PREFIX
		os.Unsetenv("TERMUX_VERSION")
		os.Setenv("PREFIX", "/data/data/com.termux/files/usr")

		if !isTermux() {
			t.Error("Should detect Termux when PREFIX contains 'com.termux'")
		}
	})
}

func TestCheckPkg(t *testing.T) {
	t.Run("should not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("checkPkg panicked: %v", r)
			}
		}()
		_ = checkPkg()
	})
}

func TestDetectTermuxFields(t *testing.T) {
	t.Run("SystemInfo should have Termux fields", func(t *testing.T) {
		info := Detect()
		// Just verify the fields exist and are initialized
		_ = info.IsTermux
		_ = info.HasPkg
		_ = info.Prefix
	})

	t.Run("Non-Termux system should have IsTermux=false", func(t *testing.T) {
		// Skip this test if we're actually running in Termux
		// (the directory /data/data/com.termux will exist regardless of env vars)
		if _, err := os.Stat("/data/data/com.termux"); err == nil {
			t.Skip("Skipping test: running in actual Termux environment")
		}

		// Save original values
		originalVersion := os.Getenv("TERMUX_VERSION")
		originalPrefix := os.Getenv("PREFIX")
		defer func() {
			if originalVersion != "" {
				os.Setenv("TERMUX_VERSION", originalVersion)
			}
			if originalPrefix != "" {
				os.Setenv("PREFIX", originalPrefix)
			}
		}()

		// Clear Termux env vars
		os.Unsetenv("TERMUX_VERSION")
		os.Setenv("PREFIX", "/usr/local") // Non-termux prefix

		// On non-Termux systems, IsTermux should be false
		info := Detect()
		if info.IsTermux {
			t.Error("IsTermux should be false on non-Termux systems")
		}
	})
}

// Helper to check if string contains termux
func containsTermux(s string) bool {
	return len(s) > 0 && (s == "/data/data/com.termux/files/usr" ||
		(len(s) > 10 && s[:10] == "/data/data"))
}
