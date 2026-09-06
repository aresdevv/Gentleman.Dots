package system

import "sync/atomic"

// dryRunFlag tracks whether the installer should avoid performing any real
// mutating operation (package manager calls, sudo, network access, file
// writes/patches, backups, shell changes, temp script creation, ...) and
// instead only report what it would do. It is process-wide because the
// interactive TUI, the non-interactive CLI path, and the low-level exec
// helpers all need to observe the same state.
var dryRunFlag atomic.Bool

// SetDryRun enables or disables dry-run mode process-wide.
func SetDryRun(enabled bool) {
	dryRunFlag.Store(enabled)
}

// IsDryRun reports whether dry-run mode is currently active.
func IsDryRun() bool {
	return dryRunFlag.Load()
}
