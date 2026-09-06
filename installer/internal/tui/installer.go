package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Gentleman-Programming/Gentleman.Dots/installer/internal/system"
)

// gentlemanDotsRemoteSubstr identifies the installer's own clone of this
// repository by its "origin" remote, so a CWD-relative "Gentleman.Dots"
// directory that merely shares the name is never mistaken for it before
// deletion. See issue #193. Kept in sync with the clone URL below (this is
// a personal fork build, so it points at aresdevv's fork, not upstream).
const gentlemanDotsRemoteSubstr = "aresdevv/gentleman.dots"

// StepError provides context about which step failed and why
type StepError struct {
	StepID      string
	StepName    string
	Description string
	Cause       error
}

func (e *StepError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Step '%s' failed\n", e.StepName))
	sb.WriteString(fmt.Sprintf("Description: %s\n", e.Description))
	if e.Cause != nil {
		sb.WriteString(fmt.Sprintf("\nDetails:\n%v", e.Cause))
	}
	return sb.String()
}

func (e *StepError) Unwrap() error {
	return e.Cause
}

// wrapStepError creates a detailed error for a step failure
func wrapStepError(stepID, stepName, description string, cause error) error {
	return &StepError{
		StepID:      stepID,
		StepName:    stepName,
		Description: description,
		Cause:       cause,
	}
}

// executeStep runs the actual installation for a step
func executeStep(stepID string, m *Model) error {
	// Dry-run: never perform a mutating operation (package manager calls,
	// sudo, network access, repo clone, config copy/patch, backups, shell
	// change, temp script creation, ...). Report the planned step instead.
	if m.DryRun {
		SendLog(stepID, fmt.Sprintf("[dry-run] Would run step '%s' — no changes made", stepID))
		return nil
	}

	switch stepID {
	case "backup":
		return stepBackupConfigs(m)
	case "clone":
		return stepCloneRepo(m)
	case "homebrew":
		return stepInstallHomebrew(m)
	case "deps":
		return stepInstallDeps(m)
	case "xcode":
		return stepInstallXcode(m)
	case "terminal":
		return stepInstallTerminal(m)
	case "font":
		return stepInstallFont(m)
	case "shell":
		return stepInstallShell(m)
	case "wm":
		return stepInstallWM(m)
	case "nvim":
		return stepInstallNvim(m)
	case "cleanup":
		return stepCleanup(m)
	case "setshell":
		return stepSetDefaultShell(m)
	default:
		return fmt.Errorf("unknown step: %s", stepID)
	}
}

func stepBackupConfigs(m *Model) error {
	stepID := "backup"
	if len(m.ExistingConfigs) == 0 {
		SendLog(stepID, "No existing configs to backup")
		return nil
	}

	SendLog(stepID, fmt.Sprintf("Backing up %d existing configs...", len(m.ExistingConfigs)))

	// Extract just the config keys from the ExistingConfigs slice
	configKeys := make([]string, len(m.ExistingConfigs))
	for i, config := range m.ExistingConfigs {
		configKeys[i] = config
		SendLog(stepID, fmt.Sprintf("  → %s", config))
	}

	backupDir, err := system.CreateBackup(configKeys)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	m.BackupDir = backupDir
	SendLog(stepID, fmt.Sprintf("✓ Backup created at: %s", backupDir))
	return nil
}

func stepCloneRepo(m *Model) error {
	stepID := "clone"

	// Check if already exists
	if _, err := os.Stat("Gentleman.Dots"); err == nil {
		SendLog(stepID, "Removing existing Gentleman.Dots directory...")
		if err := system.SafeRemoveClone("Gentleman.Dots", gentlemanDotsRemoteSubstr); err != nil {
			return wrapStepError("clone", "Clone Repository",
				"A directory named 'Gentleman.Dots' already exists here but does not look like a "+
					"disposable clone of this repository (wrong remote, local changes, or unpublished "+
					"commits). Move or remove it manually, then re-run the installer from a directory "+
					"that doesn't contain unrelated data named 'Gentleman.Dots'.",
				err)
		}
	}

	SendLog(stepID, "Cloning repository from GitHub...")
	result := system.RunWithLogs("git clone --progress --branch personal/all-fixes https://github.com/aresdevv/Gentleman.Dots.git Gentleman.Dots", nil, func(line string) {
		SendLog(stepID, line)
	})
	if result.Error != nil {
		return wrapStepError("clone", "Clone Repository",
			"Failed to clone the repository. Check your internet connection and git installation.",
			result.Error)
	}

	// Verify clone was successful
	if _, err := os.Stat("Gentleman.Dots"); os.IsNotExist(err) {
		return wrapStepError("clone", "Clone Repository",
			"Repository was cloned but directory not found",
			fmt.Errorf("Gentleman.Dots directory does not exist after clone"))
	}

	SendLog(stepID, "✓ Repository cloned successfully")
	return nil
}

func stepInstallHomebrew(m *Model) error {
	stepID := "homebrew"

	// Termux doesn't use Homebrew - it uses pkg
	if m.SystemInfo.IsTermux {
		SendLog(stepID, "Skipping Homebrew (Termux uses pkg package manager)")
		return nil
	}

	if system.CommandExists("brew") {
		SendLog(stepID, "Homebrew already installed, skipping...")
		return nil
	}

	SendLog(stepID, "Installing Homebrew package manager...")
	result := system.RunWithLogs(`/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`, nil, func(line string) {
		SendLog(stepID, line)
	})
	if result.Error != nil {
		return wrapStepError("homebrew", "Install Homebrew",
			"Failed to install Homebrew package manager. Check your internet connection.",
			result.Error)
	}

	// Add to PATH
	homeDir := os.Getenv("HOME")
	brewPrefix := system.GetBrewPrefix()

	shellConfig := fmt.Sprintf(`eval "$(%s/bin/brew shellenv)"`, brewPrefix)

	SendLog(stepID, "Configuring shell to use Homebrew...")
	// Add to common shell configs
	for _, rcFile := range []string{".bashrc", ".zshrc"} {
		rcPath := filepath.Join(homeDir, rcFile)
		if f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			f.WriteString("\n" + shellConfig + "\n")
			f.Close()
		}
	}

	// Source it now
	system.Run(shellConfig, nil)

	SendLog(stepID, "✓ Homebrew installed successfully")
	return nil
}

func stepInstallDeps(m *Model) error {
	stepID := "deps"

	// Termux: use pkg (no sudo needed)
	// Check both SystemInfo and Choices.OS for redundancy
	isTermux := m.SystemInfo.IsTermux || m.Choices.OS == "termux"
	if isTermux {
		SendLog(stepID, "Updating Termux packages...")
		result := system.RunPkgWithLogs("update", nil, func(line string) {
			SendLog(stepID, line)
		})
		if result.Error != nil {
			return wrapStepError("deps", "Install Dependencies",
				"Failed to update Termux packages",
				result.Error)
		}
		result = system.RunPkgWithLogs("upgrade -y", nil, func(line string) {
			SendLog(stepID, line)
		})
		if result.Error != nil {
			// Upgrade failures are not critical
			SendLog(stepID, "Warning: package upgrade had issues, continuing...")
		}
		SendLog(stepID, "Installing base dependencies...")
		result = system.RunPkgInstall("git curl", nil, func(line string) {
			SendLog(stepID, line)
		})
		if result.Error != nil {
			return wrapStepError("deps", "Install Dependencies",
				"Failed to install base dependencies on Termux",
				result.Error)
		}
		return nil
	}

	// Atomic/immutable distro (Silverblue, Kinoite, Bazzite, uBlue, ...):
	// the root filesystem is read-only, so sudo pacman/dnf/apt-get are not
	// an option. Prefer Homebrew (userspace, no sudo) and otherwise defer to
	// a manual rpm-ostree layering step the user runs themselves.
	if m.SystemInfo.IsAtomic {
		SendLog(stepID, "Atomic distro detected — read-only root, skipping system package manager")
		if m.SystemInfo.HasBrew {
			SendLog(stepID, "Installing base dependencies via Homebrew...")
			result := system.RunBrewWithLogs("install git curl wget unzip fontconfig", nil, func(line string) {
				SendLog(stepID, line)
			})
			if result.Error != nil {
				// Non-fatal: these tools are commonly preinstalled on Fedora
				// Atomic base images already.
				SendLog(stepID, "Warning: some Homebrew packages failed (they may already be provided by the base image)")
			}
		} else {
			SendLog(stepID, "Homebrew not available — skipping base package installation")
			m.AddManualStep("rpm-ostree install git curl wget unzip fontconfig (reboot required), or install Homebrew first")
		}
		return nil
	}

	// Arch Linux
	if m.SystemInfo.OS == system.OSArch {
		// Omarchy ships a pacman hook that aborts any transaction combining
		// -S and -u (a full system upgrade), because Omarchy wants system
		// upgrades to go through its own `omarchy update` (snapshots,
		// keyrings, Hyprland reload, etc). Installing specific packages with
		// plain `-S --needed` (no -u) is unaffected, so skip the upgrade and
		// go straight to installing what's actually needed.
		if m.SystemInfo.IsOmarchy {
			SendLog(stepID, "Omarchy detected — skipping 'pacman -Syu' (use 'omarchy update' separately); installing only the required packages.")
		} else {
			result := system.RunSudo("pacman -Syu --noconfirm", nil)
			if result.Error != nil {
				return wrapStepError("deps", "Install Dependencies",
					"Failed to update Arch Linux packages",
					result.Error)
			}
		}
		result := system.RunSudo("pacman -S --needed --noconfirm base-devel curl file git wget unzip fontconfig", nil)
		if result.Error != nil {
			return wrapStepError("deps", "Install Dependencies",
				"Failed to install base dependencies on Arch Linux",
				result.Error)
		}
		return nil
	}

	// Fedora/RHEL
	if m.SystemInfo.OS == system.OSFedora {
		result := system.RunSudo("dnf check-update || true", nil) // dnf check-update returns 100 if updates available
		// "@development-tools" is DNF4 shorthand and is not recognized by
		// DNF5 (the default dnf on Fedora since Fedora 41, including
		// Fedora 44): it must be installed via the "group install"
		// subcommand instead, and kept out of the plain package-install
		// command below since DNF5 rejects mixing the two forms.
		result = system.RunSudo(`dnf group install -y "Development Tools"`, nil)
		if result.Error != nil {
			return wrapStepError("deps", "Install Dependencies",
				"Failed to install the Development Tools group on Fedora/RHEL",
				result.Error)
		}
		// wget was retired from Fedora's repos in favor of wget2; the
		// wget2-wget subpackage provides the "wget" binary for
		// compatibility with anything that still expects it.
		result = system.RunSudo("dnf install -y curl file git wget2-wget unzip fontconfig", nil)
		if result.Error != nil {
			return wrapStepError("deps", "Install Dependencies",
				"Failed to install base dependencies on Fedora/RHEL",
				result.Error)
		}
		return nil
	}

	// Debian/Ubuntu
	result := system.RunSudo("apt-get update", nil)
	if result.Error != nil {
		return wrapStepError("deps", "Install Dependencies",
			"Failed to update apt package list",
			result.Error)
	}
	result = system.RunSudo("apt-get install -y build-essential curl file git unzip fontconfig procps", nil)
	if result.Error != nil {
		return wrapStepError("deps", "Install Dependencies",
			"Failed to install base dependencies on Debian/Ubuntu",
			result.Error)
	}
	return nil
}

func stepInstallXcode(m *Model) error {
	result := system.Run("xcode-select --install", nil)
	if result.Error != nil {
		// xcode-select returns error if already installed, which is fine
		if result.ExitCode == 1 && strings.Contains(result.Stderr, "already installed") {
			return nil
		}
		return wrapStepError("xcode", "Install Xcode CLI",
			"Failed to install Xcode Command Line Tools. You may need to install them manually from the App Store.",
			result.Error)
	}
	return nil
}

func stepInstallTerminal(m *Model) error {
	terminal := m.Choices.Terminal
	homeDir := os.Getenv("HOME")
	repoDir := "Gentleman.Dots"
	stepID := "terminal"

	if m.SystemInfo.IsAtomic {
		return stepInstallTerminalAtomic(m, terminal, homeDir, repoDir, stepID)
	}

	switch terminal {
	case "alacritty":
		if !system.CommandExists("alacritty") {
			SendLog(stepID, "Installing Alacritty...")
			var result *system.ExecResult
			if m.SystemInfo.OS == system.OSArch {
				result = system.RunSudoWithLogs("pacman -S --noconfirm alacritty", nil, func(line string) {
					SendLog(stepID, line)
				})
			} else if m.SystemInfo.OS == system.OSMac {
				result = system.RunBrewWithLogs("install --cask alacritty", nil, func(line string) {
					SendLog(stepID, line)
				})
			} else if m.SystemInfo.OS == system.OSFedora {
				// Fedora: install from dnf
				result = system.RunSudoWithLogs("dnf install -y alacritty", nil, func(line string) {
					SendLog(stepID, line)
				})
			} else if m.SystemInfo.OS == system.OSDebian || m.SystemInfo.OS == system.OSLinux {
				// Debian/Ubuntu: compile from source (PPAs are unreliable)
				SendLog(stepID, "Building Alacritty from source...")
				SendLog(stepID, "Installing build dependencies...")
				result = system.RunSudoWithLogs("apt-get install -y cmake pkg-config libfreetype6-dev libfontconfig1-dev libxcb-xfixes0-dev libxkbcommon-dev python3 gzip scdoc git curl", nil, func(line string) {
					SendLog(stepID, line)
				})
				if result.Error != nil {
					return wrapStepError("terminal", "Install Alacritty",
						"Failed to install build dependencies",
						result.Error)
				}
				// Install Rust/Cargo only for this build
				cargoPath := filepath.Join(homeDir, ".cargo/bin/cargo")
				if !system.CommandExists("cargo") && !system.CommandExists(cargoPath) {
					SendLog(stepID, "Installing Rust/Cargo toolchain...")
					result = system.RunWithLogs("curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y", nil, func(line string) {
						SendLog(stepID, line)
					})
					if result.Error != nil {
						return wrapStepError("terminal", "Install Alacritty",
							"Failed to install Rust",
							result.Error)
					}
					cargoPath = filepath.Join(homeDir, ".cargo/bin/cargo")
				}
				// Clone and build Alacritty
				SendLog(stepID, "Cloning Alacritty repository...")
				alacrittyDir := filepath.Join(os.TempDir(), "alacritty-build")
				os.RemoveAll(alacrittyDir)
				result = system.RunWithLogs(fmt.Sprintf("git clone https://github.com/alacritty/alacritty.git %s", alacrittyDir), nil, func(line string) {
					SendLog(stepID, line)
				})
				if result.Error != nil {
					return wrapStepError("terminal", "Install Alacritty",
						"Failed to clone Alacritty repository",
						result.Error)
				}
				SendLog(stepID, "Building Alacritty (this may take 5-10 minutes)...")
				if !system.CommandExists("cargo") {
					cargoPath = filepath.Join(homeDir, ".cargo/bin/cargo")
				} else {
					cargoPath = "cargo"
				}
				result = system.RunWithLogs(fmt.Sprintf("%s build --release --manifest-path %s/Cargo.toml", cargoPath, alacrittyDir), nil, func(line string) {
					SendLog(stepID, line)
				})
				if result.Error != nil {
					return wrapStepError("terminal", "Install Alacritty",
						"Failed to build Alacritty",
						result.Error)
				}
				SendLog(stepID, "Installing Alacritty binary...")
				result = system.RunSudoWithLogs(fmt.Sprintf("cp %s/target/release/alacritty /usr/local/bin/alacritty", alacrittyDir), nil, func(line string) {
					SendLog(stepID, line)
				})
				if result.Error != nil {
					return wrapStepError("terminal", "Install Alacritty",
						"Failed to install Alacritty binary",
						result.Error)
				}
				system.RunSudoWithLogs(fmt.Sprintf("cp %s/extra/linux/Alacritty.desktop /usr/share/applications/", alacrittyDir), nil, func(line string) {
					SendLog(stepID, line)
				})
				os.RemoveAll(alacrittyDir)
				SendLog(stepID, "✓ Alacritty built and installed from source")
			} else {
				return wrapStepError("terminal", "Install Alacritty",
					"Unsupported operating system for Alacritty installation",
					fmt.Errorf("OS type: %v", m.SystemInfo.OS))
			}
			if result.Error != nil {
				return wrapStepError("terminal", "Install Alacritty",
					"Failed to install Alacritty terminal emulator",
					result.Error)
			}
		} else {
			SendLog(stepID, "Alacritty already installed")
		}
		SendLog(stepID, "Copying Alacritty configuration...")
		if err := system.EnsureDir(filepath.Join(homeDir, ".config/alacritty")); err != nil {
			return wrapStepError("terminal", "Install Alacritty",
				"Failed to create Alacritty config directory",
				err)
		}
		if err := system.CopyFile(filepath.Join(repoDir, "alacritty.toml"), filepath.Join(homeDir, ".config/alacritty/alacritty.toml")); err != nil {
			return wrapStepError("terminal", "Install Alacritty",
				"Failed to copy Alacritty configuration",
				err)
		}
		SendLog(stepID, "✓ Alacritty configured")

	case "wezterm":
		if !system.CommandExists("wezterm") {
			SendLog(stepID, "Installing WezTerm...")
			var result *system.ExecResult
			if m.SystemInfo.OS == system.OSArch {
				result = system.RunSudoWithLogs("pacman -S --noconfirm wezterm", nil, func(line string) {
					SendLog(stepID, line)
				})
			} else if m.SystemInfo.OS == system.OSFedora {
				// Fedora: enable COPR and install
				system.RunSudo("dnf copr enable -y wezfurlong/wezterm-nightly", nil)
				result = system.RunSudoWithLogs("dnf install -y wezterm", nil, func(line string) {
					SendLog(stepID, line)
				})
			} else if m.SystemInfo.OS == system.OSMac {
				result = system.RunBrewWithLogs("install --cask wezterm", nil, func(line string) {
					SendLog(stepID, line)
				})
			} else {
				system.Run("brew tap wez/wezterm-linuxbrew", nil)
				result = system.RunBrewWithLogs("install wezterm", nil, func(line string) {
					SendLog(stepID, line)
				})
			}
			if result.Error != nil {
				return wrapStepError("terminal", "Install WezTerm",
					"Failed to install WezTerm terminal emulator",
					result.Error)
			}
		} else {
			SendLog(stepID, "WezTerm already installed")
		}
		SendLog(stepID, "Copying WezTerm configuration...")
		if err := system.EnsureDir(filepath.Join(homeDir, ".config/wezterm")); err != nil {
			return wrapStepError("terminal", "Install WezTerm",
				"Failed to create WezTerm config directory",
				err)
		}
		if err := system.CopyFile(filepath.Join(repoDir, ".wezterm.lua"), filepath.Join(homeDir, ".config/wezterm/wezterm.lua")); err != nil {
			return wrapStepError("terminal", "Install WezTerm",
				"Failed to copy WezTerm configuration",
				err)
		}
		SendLog(stepID, "✓ WezTerm configured")

	case "kitty":
		if !system.CommandExists("kitty") {
			SendLog(stepID, "Installing Kitty...")
			var result *system.ExecResult
			switch m.SystemInfo.OS {
			case system.OSMac:
				result = system.RunBrewWithLogs("install --cask kitty", nil, func(line string) {
					SendLog(stepID, line)
				})
			case system.OSArch:
				result = system.RunSudoWithLogs("pacman -S --noconfirm kitty", nil, func(line string) {
					SendLog(stepID, line)
				})
			case system.OSFedora:
				result = system.RunSudoWithLogs("dnf install -y kitty", nil, func(line string) {
					SendLog(stepID, line)
				})
			case system.OSDebian, system.OSLinux:
				// Kitty ships in the official Debian/Ubuntu repositories.
				result = system.RunSudoWithLogs("apt-get install -y kitty", nil, func(line string) {
					SendLog(stepID, line)
				})
			default:
				return wrapStepError("terminal", "Install Kitty",
					"Unsupported operating system for Kitty installation",
					fmt.Errorf("OS type: %v", m.SystemInfo.OS))
			}
			if result.Error != nil {
				return wrapStepError("terminal", "Install Kitty",
					"Failed to install Kitty terminal emulator",
					result.Error)
			}
		} else {
			SendLog(stepID, "Kitty already installed")
		}
		SendLog(stepID, "Copying Kitty configuration...")
		if err := system.EnsureDir(filepath.Join(homeDir, ".config/kitty")); err != nil {
			return wrapStepError("terminal", "Install Kitty",
				"Failed to create Kitty config directory",
				err)
		}
		if err := system.CopyDir(filepath.Join(repoDir, "GentlemanKitty"), filepath.Join(homeDir, ".config", "kitty")); err != nil {
			return wrapStepError("terminal", "Install Kitty",
				"Failed to copy Kitty configuration",
				err)
		}
		SendLog(stepID, "✓ Kitty configured")

	case "ghostty":
		if !system.CommandExists("ghostty") {
			SendLog(stepID, "Installing Ghostty...")
			var result *system.ExecResult
			if m.SystemInfo.OS == system.OSArch {
				result = system.RunSudoWithLogs("pacman -S --noconfirm ghostty", nil, func(line string) {
					SendLog(stepID, line)
				})
			} else if m.SystemInfo.OS == system.OSFedora {
				// Fedora: enable COPR and install
				system.RunSudo("dnf copr enable -y pgdev/ghostty", nil)
				result = system.RunSudoWithLogs("dnf install -y ghostty", nil, func(line string) {
					SendLog(stepID, line)
				})
			} else if m.SystemInfo.OS == system.OSMac {
				result = system.RunBrewWithLogs("install --cask ghostty", nil, func(line string) {
					SendLog(stepID, line)
				})
			} else {
				result = system.RunWithLogs(`/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/mkasberg/ghostty-ubuntu/HEAD/install.sh)"`, nil, func(line string) {
					SendLog(stepID, line)
				})
			}
			if result.Error != nil {
				return wrapStepError("terminal", "Install Ghostty",
					"Failed to install Ghostty terminal emulator",
					result.Error)
			}
		} else {
			SendLog(stepID, "Ghostty already installed")
		}
		SendLog(stepID, "Copying Ghostty configuration...")
		if err := system.EnsureDir(filepath.Join(homeDir, ".config/ghostty")); err != nil {
			return wrapStepError("terminal", "Install Ghostty",
				"Failed to create Ghostty config directory",
				err)
		}
		if err := system.CopyDir(filepath.Join(repoDir, "GentlemanGhostty"), filepath.Join(homeDir, ".config", "ghostty")); err != nil {
			return wrapStepError("terminal", "Install Ghostty",
				"Failed to copy Ghostty configuration",
				err)
		}
		SendLog(stepID, "✓ Ghostty configured")
	}

	return nil
}

// atomicTerminalFlatpakIDs maps a terminal choice to its Flathub application
// ID, for terminals that publish one. Alacritty and Kitty do not currently
// ship an official Flathub package, so they are intentionally absent here.
var atomicTerminalFlatpakIDs = map[string]string{
	"wezterm": "org.wezfurlong.wezterm",
	"ghostty": "com.mitchellh.ghostty",
}

// stepInstallTerminalAtomic installs (or reports how to install) a terminal
// emulator on an atomic/immutable distro, then copies its configuration.
// The system package manager (pacman/dnf/apt-get) is never invoked here:
// read-only root makes that impossible. Homebrew (userspace) is tried
// first; Flatpak is tried next for terminals that publish one; otherwise a
// manual step is recorded and shown in the final summary. The step always
// succeeds so the config gets copied and the rest of the installation can
// continue regardless of whether the binary landed.
func stepInstallTerminalAtomic(m *Model, terminal, homeDir, repoDir, stepID string) error {
	installed := system.CommandExists(terminal)

	if !installed {
		SendLog(stepID, "Atomic distro detected — read-only root, skipping the system package manager")

		if m.SystemInfo.HasBrew {
			SendLog(stepID, fmt.Sprintf("Trying Homebrew for %s...", terminal))
			result := system.RunBrewWithLogs("install "+terminal, nil, func(line string) {
				SendLog(stepID, line)
			})
			installed = result.Error == nil
			if !installed {
				SendLog(stepID, fmt.Sprintf("Homebrew install of %s failed or is unavailable on this platform", terminal))
			}
		}

		if !installed {
			flatpakID, hasFlatpakID := atomicTerminalFlatpakIDs[terminal]
			switch {
			case hasFlatpakID && m.SystemInfo.HasFlatpak:
				SendLog(stepID, fmt.Sprintf("Trying Flatpak for %s...", terminal))
				result := system.RunFlatpakWithLogs("install -y flathub "+flatpakID, nil, func(line string) {
					SendLog(stepID, line)
				})
				installed = result.Error == nil
				if !installed {
					m.AddManualStep(fmt.Sprintf("flatpak install flathub %s", flatpakID))
				}
			case hasFlatpakID:
				m.AddManualStep(fmt.Sprintf("flatpak install flathub %s (install Flatpak first)", flatpakID))
			default:
				m.AddManualStep(fmt.Sprintf("Install %s manually — try: rpm-ostree install %s", terminal, terminal))
			}
		}

		if installed {
			SendLog(stepID, fmt.Sprintf("✓ %s installed", terminal))
		} else {
			SendLog(stepID, fmt.Sprintf("ℹ %s was not installed automatically — see the manual steps summary at the end", terminal))
		}
	} else {
		SendLog(stepID, fmt.Sprintf("%s already installed", terminal))
	}

	SendLog(stepID, fmt.Sprintf("Copying %s configuration...", terminal))
	if err := copyTerminalConfig(terminal, homeDir, repoDir); err != nil {
		return wrapStepError("terminal", "Install "+terminal,
			fmt.Sprintf("Failed to copy %s configuration", terminal), err)
	}
	SendLog(stepID, fmt.Sprintf("✓ %s configured", terminal))
	return nil
}

// copyTerminalConfig copies the Gentleman.Dots configuration for the given
// terminal into the user's home directory. It is shared by the atomic
// install path, where config files are copied regardless of whether the
// terminal binary itself could be installed automatically.
func copyTerminalConfig(terminal, homeDir, repoDir string) error {
	switch terminal {
	case "alacritty":
		if err := system.EnsureDir(filepath.Join(homeDir, ".config/alacritty")); err != nil {
			return err
		}
		return system.CopyFile(filepath.Join(repoDir, "alacritty.toml"), filepath.Join(homeDir, ".config/alacritty/alacritty.toml"))
	case "wezterm":
		if err := system.EnsureDir(filepath.Join(homeDir, ".config/wezterm")); err != nil {
			return err
		}
		return system.CopyFile(filepath.Join(repoDir, ".wezterm.lua"), filepath.Join(homeDir, ".config/wezterm/wezterm.lua"))
	case "kitty":
		if err := system.EnsureDir(filepath.Join(homeDir, ".config/kitty")); err != nil {
			return err
		}
		return system.CopyDir(filepath.Join(repoDir, "GentlemanKitty"), filepath.Join(homeDir, ".config", "kitty"))
	case "ghostty":
		if err := system.EnsureDir(filepath.Join(homeDir, ".config/ghostty")); err != nil {
			return err
		}
		return system.CopyDir(filepath.Join(repoDir, "GentlemanGhostty"), filepath.Join(homeDir, ".config", "ghostty"))
	default:
		return nil
	}
}

func stepInstallFont(m *Model) error {
	homeDir := os.Getenv("HOME")
	stepID := "font"

	// Termux: fonts work differently - copy to ~/.termux/font.ttf
	isTermux := m.SystemInfo.IsTermux || m.Choices.OS == "termux"
	if isTermux {
		SendLog(stepID, "Downloading JetBrainsMono Nerd Font for Termux...")
		termuxDir := filepath.Join(homeDir, ".termux")
		if err := system.EnsureDir(termuxDir); err != nil {
			return wrapStepError("font", "Install Nerd Font",
				"Failed to create .termux directory",
				err)
		}

		// Download a single TTF file for Termux
		result := system.RunWithLogs(fmt.Sprintf("curl -fsSL -o %s/font.ttf https://github.com/ryanoasis/nerd-fonts/raw/HEAD/patched-fonts/JetBrainsMono/Ligatures/Regular/JetBrainsMonoNerdFont-Regular.ttf", termuxDir), nil, func(line string) {
			SendLog(stepID, line)
		})
		if result.Error != nil {
			return wrapStepError("font", "Install Nerd Font",
				"Failed to download font. Check your internet connection.",
				result.Error)
		}

		SendLog(stepID, "Reloading Termux settings...")
		system.Run("termux-reload-settings", nil)
		SendLog(stepID, "✓ Font installed - restart Termux to apply")
		return nil
	}

	if m.SystemInfo.OS == system.OSMac {
		SendLog(stepID, "Installing Iosevka Term Nerd Font...")
		result := system.RunBrewWithLogs("install --cask font-iosevka-term-nerd-font", nil, func(line string) {
			SendLog(stepID, line)
		})
		if result.Error != nil {
			return wrapStepError("font", "Install Iosevka Nerd Font",
				"Failed to install font via Homebrew. Try installing manually from https://www.nerdfonts.com/",
				result.Error)
		}
		SendLog(stepID, "✓ Font installed")
		return nil
	}

	// Linux
	fontDir := filepath.Join(homeDir, ".local/share/fonts")
	SendLog(stepID, "Creating fonts directory...")
	if err := system.EnsureDir(fontDir); err != nil {
		return wrapStepError("font", "Install Iosevka Nerd Font",
			"Failed to create fonts directory",
			err)
	}

	SendLog(stepID, "Downloading Iosevka Term Nerd Font...")
	result := system.RunWithLogs(fmt.Sprintf("curl -fsSL -o %s/IosevkaTerm.zip https://github.com/ryanoasis/nerd-fonts/releases/download/v3.3.0/IosevkaTerm.zip", fontDir), nil, func(line string) {
		SendLog(stepID, line)
	})
	if result.Error != nil {
		return wrapStepError("font", "Install Iosevka Nerd Font",
			"Failed to download font. Check your internet connection.",
			result.Error)
	}

	SendLog(stepID, "Extracting font archive...")
	result = system.RunWithLogs(fmt.Sprintf("unzip -o %s/IosevkaTerm.zip -d %s/", fontDir, fontDir), nil, func(line string) {
		SendLog(stepID, line)
	})
	if result.Error != nil {
		return wrapStepError("font", "Install Iosevka Nerd Font",
			"Failed to extract font archive",
			result.Error)
	}

	SendLog(stepID, "Updating font cache...")
	system.RunWithLogs("fc-cache -fv", nil, func(line string) {
		SendLog(stepID, line)
	})
	SendLog(stepID, "✓ Font installed")
	return nil
}

type platformPackages struct {
	Termux string
	Brew   string
	// Arch lists packages available in the official Arch repos (installed
	// via pacman). AUR-only packages must go in ArchAUR instead: pacman
	// transactions are atomic, so a single unresolvable name aborts the
	// whole install and nothing gets installed, even the valid packages.
	Arch string
	// ArchAUR lists AUR-only packages, installed via a detected helper
	// (yay or paru). Installing an AUR helper automatically is out of
	// scope: if none is found, these are skipped with a non-fatal warning
	// instead of failing the step.
	ArchAUR string
	// Fedora lists packages available in Fedora's official dnf repos.
	Fedora string
	// FedoraExtra lists packages NOT available in Fedora's official repos
	// (Fedora has no AUR-like helper). dnf transactions are atomic like
	// pacman's, so these must never be mixed into Fedora: a single
	// unresolvable name would abort the whole install, including the
	// otherwise-valid packages. These are installed via Homebrew when
	// available, or skipped with a non-fatal warning otherwise.
	FedoraExtra string
	Debian      string
}

var (
	runPkgInstallWithLogs = system.RunPkgInstall
	runSudoWithLogs       = system.RunSudoWithLogs
	runBrewWithLogs       = system.RunBrewWithLogs
	runAURWithLogs        = system.RunWithLogs
	detectAURHelper       = system.DetectAURHelper
)

func installPlatformPackages(m *Model, stepID string, packages platformPackages, onLog func(string)) *system.ExecResult {
	switch {
	case m.SystemInfo.IsTermux:
		return runPkgInstallWithLogs(packages.Termux, nil, onLog)
	case m.SystemInfo.IsAtomic:
		return installPlatformPackagesAtomic(m, packages, onLog)
	case m.SystemInfo.OS == system.OSArch && (packages.Arch != "" || packages.ArchAUR != ""):
		return installArchPackages(m, packages, onLog)
	case m.SystemInfo.OS == system.OSFedora && (packages.Fedora != "" || packages.FedoraExtra != ""):
		return installFedoraPackages(m, packages, onLog)
	case (m.SystemInfo.OS == system.OSDebian || m.SystemInfo.OS == system.OSLinux) && !m.SystemInfo.HasBrew && packages.Debian != "":
		return runSudoWithLogs("apt-get install -y "+packages.Debian, nil, onLog)
	default:
		if m.SystemInfo.HasBrew && packages.Brew != "" {
			return runBrewWithLogs("install "+packages.Brew, nil, onLog)
		}
		return &system.ExecResult{
			Error: fmt.Errorf("no package manager available for this platform"),
		}
	}
}

// installPlatformPackagesAtomic installs packages for shell plugins, window
// managers, and Neovim tooling on an atomic/immutable distro. Read-only root
// rules out pacman/dnf/apt-get entirely, so Homebrew (userspace) is the only
// automated path; anything it can't cover is recorded as a manual step
// instead of failing the install outright.
func installPlatformPackagesAtomic(m *Model, packages platformPackages, onLog func(string)) *system.ExecResult {
	if m.SystemInfo.HasBrew && packages.Brew != "" {
		onLog("Atomic distro detected — installing via Homebrew (userspace, no sudo required)")
		return runBrewWithLogs("install "+packages.Brew, nil, onLog)
	}

	manual := packages.Fedora
	if manual == "" {
		manual = packages.Debian
	}
	if manual == "" {
		manual = packages.Arch
	}

	onLog("Atomic distro detected — read-only root, skipping system package installation")
	if manual != "" {
		onLog("  Install manually after this run: rpm-ostree install " + manual)
		m.AddManualStep("rpm-ostree install " + manual + " (reboot required)")
	}
	if packages.Brew != "" {
		onLog("  Or install Homebrew first, then: brew install " + packages.Brew)
		m.AddManualStep("brew install " + packages.Brew)
	}

	// Not fatal: config files still get copied by the caller. There is
	// nothing further this step can do automatically without sudo.
	return &system.ExecResult{}
}

func runNativeWithBrewFallback(nativeCommand string, brewPackages string, hasBrew bool, onLog func(string)) *system.ExecResult {
	result := runSudoWithLogs(nativeCommand, nil, onLog)
	if result.Error == nil || !hasBrew || brewPackages == "" {
		return result
	}

	return runBrewWithLogs("install "+brewPackages, nil, onLog)
}

// installArchPackages installs the official-repo packages via pacman (with
// the existing brew fallback if pacman fails and Homebrew is available),
// then separately handles any AUR-only packages. AUR packages are NEVER
// passed to pacman: they are installed via a detected AUR helper, or
// skipped with a non-fatal warning if no helper is present.
func installArchPackages(m *Model, packages platformPackages, onLog func(string)) *system.ExecResult {
	result := &system.ExecResult{}
	if packages.Arch != "" {
		result = runNativeWithBrewFallback("pacman -S --needed --noconfirm "+packages.Arch, packages.Brew, m.SystemInfo.HasBrew, onLog)
		if result.Error != nil {
			return result
		}
	}

	if packages.ArchAUR != "" {
		installArchAURPackages(packages.ArchAUR, onLog)
	}

	return result
}

// installFedoraPackages installs the official-repo packages via dnf (with
// the existing brew fallback if dnf fails and Homebrew is available), then
// separately handles packages not available in Fedora's repos at all.
// Packages not in Fedora's repos are NEVER passed to dnf: a single
// unresolvable name aborts the whole dnf transaction, taking down the
// otherwise-valid packages with it.
func installFedoraPackages(m *Model, packages platformPackages, onLog func(string)) *system.ExecResult {
	result := &system.ExecResult{}
	if packages.Fedora != "" {
		result = runNativeWithBrewFallback("dnf install -y "+packages.Fedora, packages.Brew, m.SystemInfo.HasBrew, onLog)
		if result.Error != nil {
			return result
		}
	}

	if packages.FedoraExtra != "" {
		installFedoraExtraPackages(m, packages.FedoraExtra, onLog)
	}

	return result
}

// installArchAURPackages installs AUR-only packages using a detected AUR
// helper (yay or paru). Installing an AUR helper automatically is out of
// scope for this installer: if none is found, this logs a clear non-fatal
// warning and skips the AUR-only packages instead of aborting the step.
func installArchAURPackages(aurPackages string, onLog func(string)) {
	helper := detectAURHelper()
	if helper == "" {
		if onLog != nil {
			onLog(fmt.Sprintf("Warning: no AUR helper (yay/paru) found, skipping AUR-only packages: %s", aurPackages))
			onLog(fmt.Sprintf("Install an AUR helper and run: <helper> -S --needed %s", aurPackages))
		}
		return
	}

	result := runAURWithLogs(fmt.Sprintf("%s -S --needed --noconfirm %s", helper, aurPackages), nil, onLog)
	if result.Error != nil && onLog != nil {
		onLog(fmt.Sprintf("Warning: failed to install AUR packages via %s: %v", helper, result.Error))
	}
}

// installFedoraExtraPackages installs packages that are not available in
// Fedora's official dnf repositories (e.g. starship, carapace, lazygit,
// zellij: none of them are packaged for Fedora as of Fedora 44). Fedora has
// no AUR-like helper, so these are installed via Homebrew when available;
// otherwise this logs a non-fatal warning and skips them instead of
// aborting the step.
func installFedoraExtraPackages(m *Model, extraPackages string, onLog func(string)) {
	if m.SystemInfo.HasBrew {
		result := runBrewWithLogs("install "+extraPackages, nil, onLog)
		if result.Error != nil && onLog != nil {
			onLog(fmt.Sprintf("Warning: failed to install packages via brew: %v", result.Error))
		}
		return
	}

	if onLog != nil {
		onLog(fmt.Sprintf("Warning: not available via dnf on Fedora and Homebrew is not installed, skipping: %s", extraPackages))
		onLog("Install Homebrew or these tools manually: " + extraPackages)
	}
}

func installHerdrBinary(m *Model, stepID string) error {
	if system.CommandExists("herdr") {
		SendLog(stepID, "Herdr already installed")
		return nil
	}
	if m.SystemInfo.IsTermux {
		return fmt.Errorf("herdr is not available through the Termux package installer")
	}
	if m.SystemInfo.OS == system.OSMac || m.SystemInfo.HasBrew {
		result := system.RunBrewWithLogs("install herdr", nil, func(line string) {
			SendLog(stepID, line)
		})
		return result.Error
	}

	assetArch := ""
	expectedSHA256 := ""
	switch runtime.GOARCH {
	case "amd64":
		assetArch = "x86_64"
		expectedSHA256 = "b965acaffc2c22f54b6e6c64af7cf8e98a3f4ac2622630a0599c67a4b9d8a654"
	case "arm64":
		assetArch = "aarch64"
		expectedSHA256 = "3d757ac30c631e79dc45038c3ecc6423fe13a89f9cffa0f415aedd2c27f1576c"
	default:
		return fmt.Errorf("unsupported Herdr architecture: %s", runtime.GOARCH)
	}

	homeDir := os.Getenv("HOME")
	binDir := filepath.Join(homeDir, ".local", "bin")
	if err := system.EnsureDir(binDir); err != nil {
		return err
	}

	url := fmt.Sprintf("https://github.com/ogulcancelik/herdr/releases/download/v0.7.1/herdr-linux-%s", assetArch)
	dest := filepath.Join(binDir, "herdr")
	SendLog(stepID, "Downloading Herdr release binary...")
	result := system.RunWithLogs(fmt.Sprintf("curl -fsSL %q -o %q", url, dest), nil, func(line string) {
		SendLog(stepID, line)
	})
	if result.Error != nil {
		return result.Error
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		return err
	}
	actualSHA256 := sha256.Sum256(data)
	if hex.EncodeToString(actualSHA256[:]) != expectedSHA256 {
		os.Remove(dest)
		return fmt.Errorf("Herdr checksum mismatch for %s", url)
	}

	return os.Chmod(dest, 0755)
}

// shellAutostartWM returns the multiplexer identifier to embed in the
// generated shell autostart block. On Omarchy, independent terminal windows
// (Ghostty, Foot, ...) each spawn their own interactive shell; an
// unconditional `herdr` autostart there makes every one of them attach to
// the same persistent Herdr session instead of starting independently.
// Omarchy already provides its own explicit launcher and keybinding
// (Super+Ctrl+Return) for Herdr, so the shell-level autostart is skipped in
// that case. Other multiplexers (tmux, zellij) and non-Omarchy systems are
// unaffected.
func shellAutostartWM(m *Model) string {
	if m.Choices.WindowMgr == "herdr" && m.SystemInfo.IsOmarchy {
		return "none"
	}
	return m.Choices.WindowMgr
}

// shouldInstallOhMyZsh reports whether Oh My Zsh still needs to be installed
// under homeDir. Oh My Zsh manages its own Git checkout and self-update
// cycle, so an existing installation must be left untouched rather than
// overwritten with the repo's snapshot.
func shouldInstallOhMyZsh(homeDir string) bool {
	return !system.PathExists(filepath.Join(homeDir, ".oh-my-zsh"))
}

func stepInstallShell(m *Model) error {
	homeDir := os.Getenv("HOME")
	repoDir := "Gentleman.Dots"
	shell := m.Choices.Shell
	stepID := "shell"

	// Common dependencies
	SendLog(stepID, "Creating required directories...")
	system.EnsureDir(filepath.Join(homeDir, ".config"))
	system.EnsureDir(filepath.Join(homeDir, ".cache/starship"))
	system.EnsureDir(filepath.Join(homeDir, ".cache/carapace"))
	system.EnsureDir(filepath.Join(homeDir, ".local/share/atuin"))

	switch shell {
	case "fish":
		SendLog(stepID, "Installing Fish shell and plugins...")
		result := installPlatformPackages(m, stepID, platformPackages{
			Termux:      "fish starship zoxide",
			Brew:        "fish carapace zoxide atuin starship",
			Arch:        "fish zoxide atuin starship",
			ArchAUR:     "carapace-bin",
			Fedora:      "fish zoxide atuin",
			FedoraExtra: "carapace starship",
			Debian:      "fish zoxide starship",
		}, func(line string) {
			SendLog(stepID, line)
		})
		if result.Error != nil {
			return wrapStepError("shell", "Install Fish",
				"Failed to install Fish shell and dependencies",
				result.Error)
		}
		SendLog(stepID, "Copying Fish configuration...")
		if err := system.CopyFile(filepath.Join(repoDir, "starship.toml"), filepath.Join(homeDir, ".config/starship.toml")); err != nil {
			return wrapStepError("shell", "Install Fish",
				"Failed to copy starship configuration",
				err)
		}
		// Preserve any existing personal config.fish into conf.d/ before it
		// gets overwritten by the shipped template below - CopyDir merges
		// conf.d/ rather than replacing it, so this survives the copy and
		// keeps the user's previous PATH/env/alias lines active. This is in
		// addition to, not instead of, the general pre-install backup.
		fishConfigDir := filepath.Join(homeDir, ".config", "fish")
		if preserved, err := system.PreserveFishUserConfig(fishConfigDir); err != nil {
			SendLog(stepID, fmt.Sprintf("Warning: could not preserve existing Fish configuration: %v", err))
		} else if preserved {
			SendLog(stepID, "Preserved your existing config.fish into conf.d/"+"99-gentleman-preexisting-config.fish")
		}
		if err := system.CopyDir(filepath.Join(repoDir, "GentlemanFish", "fish"), filepath.Join(homeDir, ".config", "fish")); err != nil {
			return wrapStepError("shell", "Install Fish",
				"Failed to copy Fish configuration",
				err)
		}
		// Patch config.fish based on WM choice
		SendLog(stepID, "Configuring shell for window manager...")
		if err := system.PatchFishForWM(filepath.Join(homeDir, ".config/fish/config.fish"), shellAutostartWM(m), m.Choices.InstallNvim); err != nil {
			return wrapStepError("shell", "Install Fish",
				"Failed to configure config.fish for window manager",
				err)
		}
		// Remove tmux.fish function if not using tmux
		if m.Choices.WindowMgr != "tmux" {
			os.Remove(filepath.Join(homeDir, ".config/fish/functions/tmux.fish"))
		}
		// Termux: Add fish to $PREFIX/etc/shells so tmux doesn't complain
		if m.SystemInfo.IsTermux {
			SendLog(stepID, "Adding fish to Termux shells...")
			prefix := os.Getenv("PREFIX")
			if prefix == "" {
				prefix = "/data/data/com.termux/files/usr"
			}
			shellsFile := filepath.Join(prefix, "etc", "shells")
			system.EnsureDir(filepath.Join(prefix, "etc"))
			f, err := os.OpenFile(shellsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				f.WriteString(filepath.Join(prefix, "bin", "fish") + "\n")
				f.Close()
			}
		}
		SendLog(stepID, "✓ Fish shell configured")

	case "zsh":
		SendLog(stepID, "Installing Zsh and plugins...")
		result := installPlatformPackages(m, stepID, platformPackages{
			Termux:      "zsh starship zoxide",
			Brew:        "zsh carapace zoxide atuin zsh-autosuggestions zsh-syntax-highlighting zsh-autocomplete starship",
			Arch:        "zsh zoxide atuin zsh-autosuggestions zsh-syntax-highlighting zsh-autocomplete starship",
			ArchAUR:     "carapace-bin",
			Fedora:      "zsh zoxide atuin zsh-autosuggestions zsh-syntax-highlighting",
			FedoraExtra: "carapace starship",
			Debian:      "zsh zoxide starship zsh-autosuggestions zsh-syntax-highlighting",
		}, func(line string) {
			SendLog(stepID, line)
		})
		if result.Error != nil {
			return wrapStepError("shell", "Install Zsh",
				"Failed to install Zsh and plugins",
				result.Error)
		}
		SendLog(stepID, "Copying Zsh configuration...")
		if err := system.CopyFile(filepath.Join(repoDir, "GentlemanZsh/.zshrc"), filepath.Join(homeDir, ".zshrc")); err != nil {
			return wrapStepError("shell", "Install Zsh",
				"Failed to copy .zshrc configuration",
				err)
		}
		// Patch .zshrc based on WM choice
		SendLog(stepID, "Configuring shell for window manager...")
		if err := system.PatchZshForWM(filepath.Join(homeDir, ".zshrc"), shellAutostartWM(m), m.Choices.InstallNvim); err != nil {
			return wrapStepError("shell", "Install Zsh",
				"Failed to configure .zshrc for window manager",
				err)
		}
		if err := system.CopyFile(filepath.Join(repoDir, "starship.toml"), filepath.Join(homeDir, ".config/starship.toml")); err != nil {
			return wrapStepError("shell", "Install Zsh",
				"Failed to copy Starship configuration",
				err)
		}
		// Oh My Zsh owns its own Git checkout and update cycle. Overwriting an
		// existing installation with the bundled snapshot dirties its tracked
		// files and breaks `omz update` (autostash pop conflicts with
		// upstream). Only install it when it is genuinely missing.
		if shouldInstallOhMyZsh(homeDir) {
			SendLog(stepID, "Installing Oh My Zsh...")
			ohMyZshDir := filepath.Join(homeDir, ".oh-my-zsh")
			// KEEP_ZSHRC preserves the .zshrc copied above; RUNZSH/CHSH keep
			// the official installer non-interactive. See ohmyzsh's own
			// tools/install.sh for these variables.
			result := system.RunWithLogs(fmt.Sprintf(
				`ZSH=%q RUNZSH=no CHSH=no KEEP_ZSHRC=yes sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)"`,
				ohMyZshDir), nil, func(line string) {
				SendLog(stepID, line)
			})
			if result.Error != nil {
				return wrapStepError("shell", "Install Zsh",
					"Failed to install Oh My Zsh",
					result.Error)
			}
		} else {
			SendLog(stepID, "✓ Oh My Zsh already installed, leaving it untouched")
		}
		// Termux: Add zsh to $PREFIX/etc/shells so tmux doesn't complain
		if m.SystemInfo.IsTermux {
			SendLog(stepID, "Adding zsh to Termux shells...")
			prefix := os.Getenv("PREFIX")
			if prefix == "" {
				prefix = "/data/data/com.termux/files/usr"
			}
			shellsFile := filepath.Join(prefix, "etc", "shells")
			system.EnsureDir(filepath.Join(prefix, "etc"))
			f, err := os.OpenFile(shellsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				f.WriteString(filepath.Join(prefix, "bin", "zsh") + "\n")
				f.Close()
			}
		}
		SendLog(stepID, "✓ Zsh configured with Powerlevel10k")

	case "nushell":
		SendLog(stepID, "Installing Nushell and dependencies...")
		result := installPlatformPackages(m, stepID, platformPackages{
			Termux:      "nushell starship zoxide jq",
			Brew:        "nushell carapace zoxide atuin jq bash starship",
			Arch:        "nushell zoxide atuin jq bash starship",
			ArchAUR:     "carapace-bin",
			Fedora:      "nushell zoxide atuin jq bash",
			FedoraExtra: "carapace starship",
			Debian:      "nushell zoxide jq bash starship",
		}, func(line string) {
			SendLog(stepID, line)
		})
		if result.Error != nil {
			return wrapStepError("shell", "Install Nushell",
				"Failed to install Nushell and dependencies",
				result.Error)
		}
		SendLog(stepID, "Copying Nushell configuration...")
		if err := system.CopyFile(filepath.Join(repoDir, "starship.toml"), filepath.Join(homeDir, ".config/starship.toml")); err != nil {
			return wrapStepError("shell", "Install Nushell",
				"Failed to copy starship configuration",
				err)
		}
		if err := system.CopyFile(filepath.Join(repoDir, "bash-env-json"), filepath.Join(homeDir, ".config/bash-env-json")); err != nil {
			return wrapStepError("shell", "Install Nushell",
				"Failed to copy bash-env-json",
				err)
		}
		if err := system.CopyFile(filepath.Join(repoDir, "bash-env.nu"), filepath.Join(homeDir, ".config/bash-env.nu")); err != nil {
			return wrapStepError("shell", "Install Nushell",
				"Failed to copy bash-env.nu",
				err)
		}

		var nuDir string
		if runtime.GOOS == "darwin" {
			nuDir = filepath.Join(homeDir, "Library/Application Support/nushell")
		} else {
			nuDir = filepath.Join(homeDir, ".config/nushell")
		}
		if err := system.EnsureDir(nuDir); err != nil {
			return wrapStepError("shell", "Install Nushell",
				"Failed to create Nushell config directory",
				err)
		}
		if err := system.CopyDir(filepath.Join(repoDir, "GentlemanNushell"), nuDir); err != nil {
			return wrapStepError("shell", "Install Nushell",
				"Failed to copy Nushell configuration",
				err)
		}
		// Patch config.nu based on WM choice
		SendLog(stepID, "Configuring shell for window manager...")
		if err := system.PatchNushellForWM(filepath.Join(nuDir, "config.nu"), shellAutostartWM(m)); err != nil {
			return wrapStepError("shell", "Install Nushell",
				"Failed to configure config.nu for window manager",
				err)
		}
		// Termux: Add nu to $PREFIX/etc/shells so tmux doesn't complain
		if m.SystemInfo.IsTermux {
			SendLog(stepID, "Adding nushell to Termux shells...")
			prefix := os.Getenv("PREFIX")
			if prefix == "" {
				prefix = "/data/data/com.termux/files/usr"
			}
			shellsFile := filepath.Join(prefix, "etc", "shells")
			system.EnsureDir(filepath.Join(prefix, "etc"))
			f, err := os.OpenFile(shellsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				f.WriteString(filepath.Join(prefix, "bin", "nu") + "\n")
				f.Close()
			}
		}
		SendLog(stepID, "✓ Nushell configured")
	}

	return nil
}

func stepInstallWM(m *Model) error {
	homeDir := os.Getenv("HOME")
	repoDir := "Gentleman.Dots"
	wm := m.Choices.WindowMgr
	stepID := "wm"

	switch wm {
	case "tmux":
		if !system.CommandExists("tmux") {
			SendLog(stepID, "Installing Tmux...")
			result := installPlatformPackages(m, stepID, platformPackages{
				Termux: "tmux",
				Brew:   "tmux",
				Arch:   "tmux",
				Fedora: "tmux",
				Debian: "tmux",
			}, func(line string) {
				SendLog(stepID, line)
			})
			if result.Error != nil {
				return wrapStepError("wm", "Install Tmux",
					"Failed to install Tmux",
					result.Error)
			}
		} else {
			SendLog(stepID, "Tmux already installed")
		}

		// TPM
		tpmDir := filepath.Join(homeDir, ".tmux/plugins/tpm")
		if _, err := os.Stat(tpmDir); os.IsNotExist(err) {
			SendLog(stepID, "Cloning TPM (Tmux Plugin Manager)...")
			result := system.RunWithLogs(fmt.Sprintf("git clone https://github.com/tmux-plugins/tpm %s", tpmDir), nil, func(line string) {
				SendLog(stepID, line)
			})
			if result.Error != nil {
				return wrapStepError("wm", "Install Tmux",
					"Failed to clone TPM (Tmux Plugin Manager)",
					result.Error)
			}
		}

		SendLog(stepID, "Copying Tmux configuration...")
		if err := system.EnsureDir(filepath.Join(homeDir, ".tmux")); err != nil {
			return wrapStepError("wm", "Install Tmux",
				"Failed to create .tmux directory",
				err)
		}
		if err := system.CopyDir(filepath.Join(repoDir, "GentlemanTmux", "plugins"), filepath.Join(homeDir, ".tmux", "plugins")); err != nil {
			return wrapStepError("wm", "Install Tmux",
				"Failed to copy Tmux plugins",
				err)
		}
		if err := system.CopyFile(filepath.Join(repoDir, "GentlemanTmux/tmux.conf"), filepath.Join(homeDir, ".tmux.conf")); err != nil {
			return wrapStepError("wm", "Install Tmux",
				"Failed to copy tmux.conf",
				err)
		}

		// Configure tmux to use the user's chosen shell
		SendLog(stepID, "Configuring tmux default shell...")
		tmuxConfPath := filepath.Join(homeDir, ".tmux.conf")
		shellName := ""
		switch m.Choices.Shell {
		case "fish":
			shellName = "fish"
		case "zsh":
			shellName = "zsh"
		case "nushell":
			shellName = "nu"
		}
		if shellName != "" {
			// Find the full path to the shell
			shellFullPath := ""
			if m.SystemInfo.IsTermux {
				// In Termux, construct the path directly (which command has issues)
				prefix := os.Getenv("PREFIX")
				if prefix == "" {
					prefix = "/data/data/com.termux/files/usr"
				}
				shellFullPath = filepath.Join(prefix, "bin", shellName)
			} else {
				result := system.Run(fmt.Sprintf("which %s", shellName), nil)
				if result.Error == nil && result.Output != "" {
					shellFullPath = strings.TrimSpace(result.Output)
				}
			}
			if shellFullPath == "" {
				shellFullPath = shellName // Fallback
			}

			// Replace placeholder in tmux.conf with actual shell config
			content, err := os.ReadFile(tmuxConfPath)
			if err == nil {
				shellConfig := fmt.Sprintf("set -g default-command \"%s\"\nset -g default-shell \"%s\"", shellFullPath, shellFullPath)
				newContent := strings.Replace(string(content), "# GENTLEMAN_DEFAULT_SHELL", shellConfig, 1)
				os.WriteFile(tmuxConfPath, []byte(newContent), 0644)
			}
		}

		// Install plugins
		SendLog(stepID, "Installing Tmux plugins...")
		system.RunWithLogs(filepath.Join(homeDir, ".tmux/plugins/tpm/bin/install_plugins"), nil, func(line string) {
			SendLog(stepID, line)
		})
		SendLog(stepID, "✓ Tmux configured")

	case "zellij":
		if !system.CommandExists("zellij") {
			SendLog(stepID, "Installing Zellij...")
			result := installPlatformPackages(m, stepID, platformPackages{
				Termux:      "zellij",
				Brew:        "zellij",
				Arch:        "zellij",
				FedoraExtra: "zellij",
				Debian:      "zellij",
			}, func(line string) {
				SendLog(stepID, line)
			})
			if result.Error != nil {
				return wrapStepError("wm", "Install Zellij",
					"Failed to install Zellij",
					result.Error)
			}
		} else {
			SendLog(stepID, "Zellij already installed")
		}

		SendLog(stepID, "Copying Zellij configuration...")
		zellijDir := filepath.Join(homeDir, ".config/zellij")
		if err := system.EnsureDir(zellijDir); err != nil {
			return wrapStepError("wm", "Install Zellij",
				"Failed to create Zellij config directory",
				err)
		}
		if err := system.CopyDir(filepath.Join(repoDir, "GentlemanZellij", "zellij"), zellijDir); err != nil {
			return wrapStepError("wm", "Install Zellij",
				"Failed to copy Zellij configuration",
				err)
		}

		// Configure zellij to use the user's chosen shell
		SendLog(stepID, "Configuring zellij default shell...")
		zellijConfPath := filepath.Join(zellijDir, "config.kdl")
		shellPath := ""
		switch m.Choices.Shell {
		case "fish":
			shellPath = "fish"
		case "zsh":
			shellPath = "zsh"
		case "nushell":
			shellPath = "nu"
		}
		if shellPath != "" {
			// Append default_shell config to zellij config.kdl
			f, err := os.OpenFile(zellijConfPath, os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				f.WriteString(fmt.Sprintf("\n// Default shell (configured by Gentleman.Dots)\ndefault_shell \"%s\"\n", shellPath))
				f.Close()
			}
		}
		SendLog(stepID, "✓ Zellij configured")

	case "herdr":
		if !system.CommandExists("herdr") {
			SendLog(stepID, "Installing Herdr...")
			if err := installHerdrBinary(m, stepID); err != nil {
				return wrapStepError("wm", "Install Herdr",
					"Failed to install Herdr",
					err)
			}
		} else {
			SendLog(stepID, "Herdr already installed")
		}

		SendLog(stepID, "Copying Herdr configuration...")
		herdrDir := filepath.Join(homeDir, ".config", "herdr")
		if err := system.EnsureDir(herdrDir); err != nil {
			return wrapStepError("wm", "Install Herdr",
				"Failed to create Herdr config directory",
				err)
		}
		if err := system.CopyFile(filepath.Join(repoDir, "herdr", "config.toml"), filepath.Join(herdrDir, "config.toml")); err != nil {
			return wrapStepError("wm", "Install Herdr",
				"Failed to copy Herdr configuration",
				err)
		}
		if err := os.Chmod(filepath.Join(herdrDir, "config.toml"), 0644); err != nil {
			return wrapStepError("wm", "Install Herdr",
				"Failed to make Herdr configuration writable",
				err)
		}
		SendLog(stepID, "✓ Herdr configured")
	}

	return nil
}

func stepInstallNvim(m *Model) error {
	homeDir := os.Getenv("HOME")
	repoDir := "Gentleman.Dots"
	stepID := "nvim"

	// Obsidian path
	SendLog(stepID, "Creating Obsidian directories...")
	obsidianDir := filepath.Join(homeDir, ".config/obsidian")
	system.EnsureDir(obsidianDir)
	system.EnsureDir(filepath.Join(obsidianDir, "templates"))

	// Check Node.js
	if !system.CommandExists("node") {
		SendLog(stepID, "Installing Node.js...")
		result := installPlatformPackages(m, stepID, platformPackages{
			Termux: "nodejs",
			Brew:   "node",
			Arch:   "nodejs npm",
			// Fedora has no standalone "npm" package: npm ships as the
			// separate "nodejs-npm" subpackage.
			Fedora: "nodejs nodejs-npm",
			Debian: "nodejs npm",
		}, func(line string) {
			SendLog(stepID, line)
		})
		if result.Error != nil {
			return wrapStepError("nvim", "Install Neovim",
				"Failed to install Node.js (required for LSP servers)",
				result.Error)
		}
	} else {
		SendLog(stepID, "Node.js already installed")
	}

	// Install dependencies
	SendLog(stepID, "Installing Neovim and dependencies...")
	// Termux package names differ from desktop Linux package managers.
	result := installPlatformPackages(m, stepID, platformPackages{
		Termux: "neovim git clang fzf fd ripgrep bat curl lazygit",
		Brew:   "nvim git gcc fzf fd ripgrep coreutils bat curl lazygit tree-sitter",
		Arch:   "neovim git gcc fzf fd ripgrep coreutils bat curl lazygit tree-sitter",
		// lazygit is not packaged for Fedora (only stale, unofficial COPRs
		// exist); install it via Homebrew when available instead.
		Fedora:      "neovim git gcc fzf fd-find ripgrep coreutils bat curl tree-sitter-cli",
		FedoraExtra: "lazygit",
		Debian:      "neovim git gcc fzf fd-find ripgrep coreutils bat curl lazygit tree-sitter-cli",
	}, func(line string) {
		SendLog(stepID, line)
	})
	if result.Error != nil {
		return wrapStepError("nvim", "Install Neovim",
			"Failed to install Neovim and dependencies",
			result.Error)
	}

	// Copy config
	SendLog(stepID, "Copying Neovim configuration...")
	nvimDir := filepath.Join(homeDir, ".config/nvim")
	if err := system.EnsureDir(nvimDir); err != nil {
		return wrapStepError("nvim", "Install Neovim",
			"Failed to create Neovim config directory",
			err)
	}
	// Copy nvim config directory
	srcNvim := filepath.Join(repoDir, "GentlemanNvim", "nvim")
	if err := system.CopyDir(srcNvim, nvimDir); err != nil {
		return wrapStepError("nvim", "Install Neovim",
			"Failed to copy Neovim configuration",
			err)
	}

	// Install Claude Code CLI (optional, don't fail on error)
	// Skip on Termux - Claude Code doesn't support Android
	if !m.SystemInfo.IsTermux {
		SendLog(stepID, "Installing Claude Code CLI (optional)...")
		system.RunWithLogs(`curl -fsSL https://claude.ai/install.sh | bash`, nil, func(line string) {
			SendLog(stepID, line)
		})
		// AI tool configs are managed by gentle-ai (https://github.com/gentleman-programming/gentle-ai)
	} else {
		SendLog(stepID, "Skipping Claude Code (not supported on Termux)")
	}

	// Install OpenCode CLI (optional, don't fail on error)
	// Skip on Termux - OpenCode doesn't support Android
	if !m.SystemInfo.IsTermux {
		SendLog(stepID, "Installing OpenCode CLI (optional)...")
		system.RunWithLogs(`curl -fsSL https://opencode.ai/install | bash`, nil, func(line string) {
			SendLog(stepID, line)
		})
		// AI tool configs are managed by gentle-ai (https://github.com/gentleman-programming/gentle-ai)
	} else {
		SendLog(stepID, "Skipping OpenCode (not supported on Termux)")
	}

	SendLog(stepID, "✓ Neovim configured with Gentleman setup")
	return nil
}

func stepCleanup(m *Model) error {
	stepID := "cleanup"
	SendLog(stepID, "Removing temporary files...")
	// Only remove the cloned repo - no sudo needed
	if err := system.SafeRemoveClone("Gentleman.Dots", gentlemanDotsRemoteSubstr); err != nil {
		// Non-critical error, just log it
		SendLog(stepID, "Warning: Could not remove temporary directory")
		return nil
	}
	SendLog(stepID, "✓ Cleanup complete")
	return nil
}

// stepSetDefaultShell sets the selected shell as the user's default shell
// In non-interactive mode, this handles Termux specially (via .bashrc)
// and attempts to set the shell on other systems if possible
func stepSetDefaultShell(m *Model) error {
	stepID := "setshell"
	shell := m.Choices.Shell
	homeDir := os.Getenv("HOME")

	var shellCmd string
	switch shell {
	case "fish":
		shellCmd = "fish"
	case "zsh":
		shellCmd = "zsh"
	case "nushell":
		shellCmd = "nu"
	default:
		SendLog(stepID, fmt.Sprintf("Unknown shell: %s, skipping", shell))
		return nil
	}

	// Termux: no chsh available, modify .bashrc to auto-start shell
	if m.SystemInfo.IsTermux {
		SendLog(stepID, "Configuring shell auto-start for Termux...")

		// Find the shell path
		shellPath := system.Run(fmt.Sprintf("which %s", shellCmd), nil)
		if shellPath.Error != nil || strings.TrimSpace(shellPath.Output) == "" {
			SendLog(stepID, fmt.Sprintf("Shell '%s' not found in PATH, skipping", shellCmd))
			return nil
		}
		shellPathStr := strings.TrimSpace(shellPath.Output)

		// Read existing .bashrc
		bashrcPath := filepath.Join(homeDir, ".bashrc")
		existingContent := ""
		if data, err := os.ReadFile(bashrcPath); err == nil {
			existingContent = string(data)
		}

		// Check if already configured
		if strings.Contains(existingContent, "# Gentleman.Dots shell auto-start") {
			SendLog(stepID, "Shell auto-start already configured in ~/.bashrc")
			return nil
		}

		// Append auto-start configuration
		autoStartConfig := fmt.Sprintf(`
# Gentleman.Dots shell auto-start
if [ -x "%s" ] && [ -z "$GENTLEMANDOTS_SHELL_STARTED" ]; then
    export GENTLEMANDOTS_SHELL_STARTED=1
    exec %s
fi
`, shellPathStr, shellPathStr)

		f, err := os.OpenFile(bashrcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return wrapStepError("setshell", "Set Default Shell",
				"Failed to open ~/.bashrc for writing",
				err)
		}
		defer f.Close()

		if _, err := f.WriteString(autoStartConfig); err != nil {
			return wrapStepError("setshell", "Set Default Shell",
				"Failed to write shell auto-start to ~/.bashrc",
				err)
		}

		SendLog(stepID, fmt.Sprintf("✓ Configured %s to auto-start in ~/.bashrc", shell))
		SendLog(stepID, "Close and reopen Termux for changes to take effect")
		return nil
	}

	// Atomic/immutable distro: /etc/shells is on the read-only base image, so
	// neither `usermod` nor `chsh` can persist a new default shell there.
	// Report the manual command instead of attempting sudo.
	if m.SystemInfo.IsAtomic {
		SendLog(stepID, "Atomic distro detected — /etc/shells is read-only, skipping shell change")
		shellPath := system.Run(fmt.Sprintf("which %s", shellCmd), nil)
		resolvedShell := shellCmd
		if shellPath.Error == nil && strings.TrimSpace(shellPath.Output) != "" {
			resolvedShell = strings.TrimSpace(shellPath.Output)
		}
		SendLog(stepID, fmt.Sprintf("ℹ Set your default shell manually: chsh -s %s", resolvedShell))
		m.AddManualStep(fmt.Sprintf("chsh -s %s", resolvedShell))
		return nil
	}

	// Non-Termux: Try to set shell using sudo usermod (works if NOPASSWD configured)
	// Find the shell path first
	shellPath := system.Run(fmt.Sprintf("which %s", shellCmd), nil)
	if shellPath.Error != nil || strings.TrimSpace(shellPath.Output) == "" {
		SendLog(stepID, fmt.Sprintf("Shell '%s' not found in PATH, skipping", shellCmd))
		return nil
	}
	shellPathStr := strings.TrimSpace(shellPath.Output)

	// Get current username
	currentUser := os.Getenv("USER")
	if currentUser == "" {
		currentUser = os.Getenv("LOGNAME")
	}
	if currentUser == "" {
		// Fallback to whoami command (useful in Docker containers)
		whoamiResult := system.Run("whoami", nil)
		if whoamiResult.Error == nil {
			currentUser = strings.TrimSpace(whoamiResult.Output)
		}
	}
	if currentUser == "" {
		SendLog(stepID, "Could not determine current user, skipping shell change")
		return nil
	}

	// First, ensure shell is in /etc/shells
	SendLog(stepID, fmt.Sprintf("Adding %s to /etc/shells if needed...", shellPathStr))
	checkShells := system.Run(fmt.Sprintf("grep -q '^%s$' /etc/shells", shellPathStr), nil)
	if checkShells.Error != nil {
		// Shell not in /etc/shells, try to add it
		addResult := system.RunSudo(fmt.Sprintf("sh -c 'echo \"%s\" >> /etc/shells'", shellPathStr), nil)
		if addResult.Error != nil {
			SendLog(stepID, fmt.Sprintf("Could not add %s to /etc/shells (may need manual setup)", shellPathStr))
		}
	}

	// Try sudo usermod first (more reliable than chsh in scripts)
	SendLog(stepID, fmt.Sprintf("Setting %s as default shell for %s...", shell, currentUser))
	result := system.RunSudo(fmt.Sprintf("usermod -s %s %s", shellPathStr, currentUser), nil)
	if result.Error != nil {
		// usermod failed, try chsh as fallback
		SendLog(stepID, "usermod failed, trying chsh...")
		result = system.RunSudo(fmt.Sprintf("chsh -s %s %s", shellPathStr, currentUser), nil)
		if result.Error != nil {
			// Both failed - not critical, just inform user
			SendLog(stepID, fmt.Sprintf("Could not set default shell automatically"))
			SendLog(stepID, fmt.Sprintf("Run manually: chsh -s %s", shellPathStr))
			return nil
		}
	}

	SendLog(stepID, fmt.Sprintf("✓ Default shell set to %s", shell))
	SendLog(stepID, "Log out and log back in for changes to take effect")
	return nil
}
