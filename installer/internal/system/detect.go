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
	OS        OSType
	OSName    string
	IsWSL     bool
	IsARM     bool
	IsTermux  bool
	IsOmarchy bool // Arch-based Omarchy Linux (https://omarchy.org)
	HomeDir   string
	HasBrew   bool
	HasPkg    bool // Termux package manager
	HasXcode  bool
	UserShell string
	Prefix    string // Termux $PREFIX or empty for other systems
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

		if isOmarchy() {
			info.OS = OSArch
			info.OSName = "Omarchy"
			info.IsOmarchy = true
		} else if isArchLinux() {
			info.OS = OSArch
			info.OSName = "Arch Linux"
		} else if isFedora() {
			info.OS = OSFedora
			info.OSName = "Fedora/RHEL"
		} else if isDebian() {
			info.OS = OSDebian
			info.OSName = "Debian/Ubuntu"
		}
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

// isOmarchy detects Omarchy (https://omarchy.org), an Arch-based Linux
// distribution that ships its own terminal launchers and keybindings
// (e.g. Super+Return for a plain terminal, Super+Ctrl+Return for Herdr).
// Omarchy also has /etc/arch-release, so this must be checked before the
// generic isArchLinux() branch.
func isOmarchy() bool {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	return osReleaseIDIsOmarchy(data)
}

// osReleaseIDIsOmarchy reports whether the given /etc/os-release content
// declares ID=omarchy. Split out from isOmarchy for testability, since
// /etc/os-release itself can't be swapped out in a unit test.
func osReleaseIDIsOmarchy(osRelease []byte) bool {
	for _, line := range strings.Split(string(osRelease), "\n") {
		line = strings.TrimSpace(line)
		if line == `ID=omarchy` || line == `ID="omarchy"` {
			return true
		}
	}
	return false
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

// DetectAURHelper returns the name of an AUR helper already installed on
// this system ("yay" or "paru"), or "" if neither is found on PATH.
// Installing an AUR helper automatically is out of scope: callers should
// degrade gracefully (skip AUR-only packages with a warning) when this
// returns "".
func DetectAURHelper() string {
	for _, helper := range []string{"yay", "paru"} {
		if CommandExists(helper) {
			return helper
		}
	}
	return ""
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

// ResolveBrewCommand returns the command to invoke Homebrew with.
//
// It prefers the "brew" binary already resolvable on PATH: that is the
// same check HasBrew/checkBrew performs, so if a caller trusts HasBrew to
// decide whether to run brew at all, the actual invocation must honor the
// same resolution - otherwise a non-default install location (for example
// a per-user "$HOME/.linuxbrew" install, common when Homebrew is used
// specifically to fill gaps in a distro's native package manager) is
// genuinely on PATH but GetBrewPrefix's hardcoded guess is not, and the
// command fails with "no such file or directory" / exit 127 even though
// Homebrew is installed and working.
//
// It falls back to the well-known install-location path only when "brew"
// is not yet resolvable on PATH - this covers running brew immediately
// after installing it within the same process (stepInstallHomebrew sources
// brew's shellenv in a child shell, which does not update this process's
// own PATH).
func ResolveBrewCommand() string {
	if _, err := exec.LookPath("brew"); err == nil {
		return "brew"
	}
	return GetBrewPrefix() + "/bin/brew"
}
