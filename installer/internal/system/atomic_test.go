package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	t.Run("parses quoted and unquoted values", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "os-release")
		content := `NAME="Fedora Linux"
ID=fedora
VERSION_ID=40
VARIANT_ID="silverblue"
ID_LIKE="fedora rhel"
# a comment line should be ignored

PRETTY_NAME=Fedora
`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		fields := parseOSRelease(path)

		if fields["id"] != "" {
			t.Errorf("keys should preserve case as written, got lowercase key present")
		}
		if fields["ID"] != "fedora" {
			t.Errorf("expected ID=fedora, got %q", fields["ID"])
		}
		if fields["VARIANT_ID"] != "silverblue" {
			t.Errorf("expected VARIANT_ID=silverblue (quotes stripped), got %q", fields["VARIANT_ID"])
		}
		if fields["ID_LIKE"] != "fedora rhel" {
			t.Errorf("expected ID_LIKE='fedora rhel', got %q", fields["ID_LIKE"])
		}
		if fields["PRETTY_NAME"] != "fedora" {
			t.Errorf("expected PRETTY_NAME=fedora, got %q", fields["PRETTY_NAME"])
		}
	})

	t.Run("missing file returns empty map without error", func(t *testing.T) {
		fields := parseOSRelease(filepath.Join(t.TempDir(), "does-not-exist"))
		if len(fields) != 0 {
			t.Errorf("expected empty map for missing file, got %v", fields)
		}
	})
}

func TestIsAtomicOSRelease(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]string
		want   bool
	}{
		{"fedora silverblue via VARIANT_ID", map[string]string{"ID": "fedora", "VARIANT_ID": "silverblue"}, true},
		{"fedora kinoite via VARIANT_ID", map[string]string{"ID": "fedora", "VARIANT_ID": "kinoite"}, true},
		{"bazzite via ID", map[string]string{"ID": "bazzite"}, true},
		{"vanilla os via ID", map[string]string{"ID": "vanillaos"}, true},
		{"opensuse microos via ID", map[string]string{"ID": "microos"}, true},
		{"atomic via ID_LIKE", map[string]string{"ID": "some-derivative", "ID_LIKE": "bazzite fedora"}, true},
		{"plain fedora workstation", map[string]string{"ID": "fedora", "VARIANT_ID": "workstation"}, false},
		{"plain arch", map[string]string{"ID": "arch"}, false},
		{"empty fields", map[string]string{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAtomicOSRelease(tc.fields); got != tc.want {
				t.Errorf("isAtomicOSRelease(%v) = %v, want %v", tc.fields, got, tc.want)
			}
		})
	}
}

func TestCheckAtomicDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("checkAtomic panicked: %v", r)
		}
	}()
	_ = checkAtomic()
}

func TestCheckFlatpakDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("checkFlatpak panicked: %v", r)
		}
	}()
	_ = checkFlatpak()
}

func TestSystemInfoHasAtomicFields(t *testing.T) {
	info := Detect()
	// Just verify the fields exist and Detect() doesn't panic populating them.
	_ = info.IsAtomic
	_ = info.HasFlatpak
}
