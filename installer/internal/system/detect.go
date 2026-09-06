package system

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type OSType int

const (
	OSMac OSType = iota
	OSLinux
	OSArch
	OSDebian // Debian-based (Debian, Ubuntu, etc.)
	OSFedora // Fedora/RHEL-based (Fedora, CentOS, RHEL, etc.)
	OSTermux // Termux on Android
	OSUnknown
)

type SystemInfo struct {
	OS         OSType
	OSName     string
	IsWSL      bool
	IsARM      bool
	IsTermux   bool
	IsAtomic   bool // Atomic/immutable distro (Fedora Silverblue/Kinoite, Bazzite, uBlue, ...)
	HomeDir    string
	HasBrew    bool
	HasPkg     bool // Termux package manager
	HasXcode   bool
	HasFlatpak bool
	UserShell  string
	Prefix     string // Termux $PREFIX or empty for other systems
}

func Detect() *SystemInfo {
	info := &SystemInfo{
		OS:      OSUnknown,
		OSName:  "Unknown",
		HomeDir: os.Getenv("HOME"),
		IsARM:   runtime.GOARCH == "arm64" || runtime.GOARCH == "arm",
		Prefix:  os.Getenv("PREFIX"),
	}

	// Check for Termux FIRST (it runs on Linux but is special)
	if isTermux() {
		info.OS = OSTermux
		info.OSName = "Termux"
		info.IsTermux = true
		info.HasPkg = checkPkg()
		info.HasBrew = false // Termux doesn't use Homebrew
		info.UserShell = detectCurrentShell()
		return info
	}

	switch runtime.GOOS {
	case "darwin":
		info.OS = OSMac
		info.OSName = "macOS"
		info.HasXcode = checkXcode()
	case "linux":
		info.OS = OSLinux
		info.OSName = "Linux"
		info.IsWSL = checkWSL()

		if isArchLinux() {
			info.OS = OSArch
			info.OSName = "Arch Linux"
		} else if isFedora() {
			info.OS = OSFedora
			info.OSName = "Fedora/RHEL"
		} else if isDebian() {
			info.OS = OSDebian
			info.OSName = "Debian/Ubuntu"
		}

		info.IsAtomic = checkAtomic()
		if info.IsAtomic {
			info.OSName += " (Atomic)"
		}
		info.HasFlatpak = checkFlatpak()
	}

	info.HasBrew = checkBrew()
	info.UserShell = detectCurrentShell()

	return info
}

func checkWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	content := strings.ToLower(string(data))
	return strings.Contains(content, "microsoft") || strings.Contains(content, "wsl")
}

func isArchLinux() bool {
	_, err := os.Stat("/etc/arch-release")
	return err == nil
}

func isDebian() bool {
	_, err := os.Stat("/etc/debian_version")
	return err == nil
}

func isFedora() bool {
	// Check for Fedora specifically
	if _, err := os.Stat("/etc/fedora-release"); err == nil {
		return true
	}
	// Check for RHEL/CentOS (also use dnf)
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		return true
	}
	return false
}

// isTermux detects if we're running in Termux on Android
func isTermux() bool {
	// Check TERMUX_VERSION environment variable
	if os.Getenv("TERMUX_VERSION") != "" {
		return true
	}
	// Check PREFIX contains termux path
	prefix := os.Getenv("PREFIX")
	if strings.Contains(prefix, "com.termux") {
		return true
	}
	// Check for Termux-specific paths
	if _, err := os.Stat("/data/data/com.termux"); err == nil {
		return true
	}
	return false
}

// checkPkg checks if Termux pkg command is available
func checkPkg() bool {
	_, err := exec.LookPath("pkg")
	return err == nil
}

func checkBrew() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// atomicDistroIDs lists known /etc/os-release ID / VARIANT_ID / ID_LIKE
// values for atomic (ostree-based) Linux distributions. This is a fallback
// heuristic: /run/ostree-booted and the rpm-ostree binary (checked first in
// checkAtomic) are the reliable, distro-name-independent signals.
var atomicDistroIDs = map[string]bool{
	"silverblue":        true,
	"fedora-silverblue": true,
	"kinoite":           true,
	"fedora-kinoite":    true,
	"bazzite":           true,
	"bluefin":           true,
	"aurora":            true,
	"sericea":           true,
	"onyx":              true,
	"vanillaos":         true,
	"vanilla":           true,
	"microos":           true,
	"aeon":              true,
	"endless":           true,
	"fedora-iot":        true,
}

// checkAtomic detects whether the current system is an atomic/immutable
// Linux distribution (read-only root, image-based updates via ostree or
// similar). Detection is layered from most to least reliable:
//  1. /run/ostree-booted — the canonical runtime marker on every
//     ostree-based system, regardless of distro name.
//  2. the rpm-ostree binary — present on every Fedora Atomic derivative
//     (Silverblue, Kinoite, Bazzite, uBlue images, ...).
//  3. /etc/os-release ID / VARIANT_ID / ID_LIKE matched against a list of
//     known atomic distro identifiers (covers non-rpm-ostree systems such
//     as openSUSE MicroOS/Aeon or Endless OS).
func checkAtomic() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	if _, err := os.Stat("/run/ostree-booted"); err == nil {
		return true
	}

	if _, err := exec.LookPath("rpm-ostree"); err == nil {
		return true
	}

	return isAtomicOSRelease(parseOSRelease("/etc/os-release"))
}

// isAtomicOSRelease reports whether the given parsed os-release fields
// (as produced by parseOSRelease) identify a known atomic distro, by
// checking ID, VARIANT_ID, and each ID_LIKE entry against atomicDistroIDs.
func isAtomicOSRelease(fields map[string]string) bool {
	if atomicDistroIDs[fields["ID"]] || atomicDistroIDs[fields["VARIANT_ID"]] {
		return true
	}
	for _, id := range strings.Fields(fields["ID_LIKE"]) {
		if atomicDistroIDs[id] {
			return true
		}
	}
	return false
}

// checkFlatpak reports whether the flatpak command is available. Several
// atomic distros (Bazzite/uBlue images especially) rely on Flatpak as their
// primary GUI application distribution mechanism.
func checkFlatpak() bool {
	_, err := exec.LookPath("flatpak")
	return err == nil
}

// parseOSRelease parses an os-release-style file (KEY=VALUE per line, values
// optionally quoted) into a lowercase-value map. Missing/unreadable files
// yield an empty map; missing keys read back as "".
func parseOSRelease(path string) map[string]string {
	fields := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return fields
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[key] = strings.ToLower(strings.Trim(value, `"'`))
	}
	return fields
}

func checkXcode() bool {
	cmd := exec.Command("xcode-select", "-p")
	return cmd.Run() == nil
}

func detectCurrentShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "unknown"
	}
	parts := strings.Split(shell, "/")
	return parts[len(parts)-1]
}

// CommandExists checks if a command is available in PATH
func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// GetBrewPrefix returns the homebrew prefix path
func GetBrewPrefix() string {
	if runtime.GOOS == "darwin" {
		// Apple Silicon (arm64) uses /opt/homebrew
		// Intel (amd64) uses /usr/local
		if runtime.GOARCH == "arm64" {
			return "/opt/homebrew"
		}
		return "/usr/local"
	}
	return "/home/linuxbrew/.linuxbrew"
}
