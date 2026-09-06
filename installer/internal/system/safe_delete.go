package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SafeRemoveClone deletes the directory at path, but only after verifying it
// is actually a git clone whose "origin" remote matches expectedRemoteSubstr
// and which holds no work that exists nowhere else.
//
// This guards against `rm -rf <name>` being run against whatever happens to
// be at that (often CWD-relative) path: an unrelated directory that merely
// shares the expected name would fail the origin-remote check below, and a
// genuine old clone of this repository with local-only commits would fail
// the "reachable from a remote-tracking branch" check. See issue #193.
//
// If path does not exist, SafeRemoveClone returns nil (nothing to do).
func SafeRemoveClone(path, expectedRemoteSubstr string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", path, err)
	}

	info, statErr := os.Lstat(absPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil
		}
		return statErr
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing to delete %s: not a directory", absPath)
	}

	if err := verifyDeletableClone(absPath, expectedRemoteSubstr); err != nil {
		return fmt.Errorf("refusing to delete %s: %w", absPath, err)
	}

	return os.RemoveAll(absPath)
}

// verifyDeletableClone returns nil only when absPath is safe to destroy:
// it must be a git repository, its "origin" remote must contain
// expectedRemoteSubstr, the working tree must be clean (no modifications,
// no untracked files, no stashes), and HEAD must be reachable from at least
// one remote-tracking branch (i.e. it holds no commits that exist nowhere
// else). A clean working tree alone is not sufficient: a clean checkout can
// still sit on local-only commits.
func verifyDeletableClone(absPath, expectedRemoteSubstr string) error {
	if _, err := os.Stat(filepath.Join(absPath, ".git")); err != nil {
		return fmt.Errorf("not a git repository")
	}

	remoteURL, err := gitOutput(absPath, "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("could not read git remote 'origin': %w", err)
	}
	if expectedRemoteSubstr != "" && !strings.Contains(strings.ToLower(remoteURL), strings.ToLower(expectedRemoteSubstr)) {
		return fmt.Errorf("origin remote %q does not match the expected repository", strings.TrimSpace(remoteURL))
	}

	status, err := gitOutput(absPath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("could not read git status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("working tree is not clean")
	}

	stashes, err := gitOutput(absPath, "stash", "list")
	if err != nil {
		return fmt.Errorf("could not read git stash list: %w", err)
	}
	if strings.TrimSpace(stashes) != "" {
		return fmt.Errorf("one or more stashes are present")
	}

	containing, err := gitOutput(absPath, "branch", "-r", "--contains", "HEAD")
	if err != nil {
		return fmt.Errorf("could not verify HEAD against remote-tracking branches: %w", err)
	}
	if strings.TrimSpace(containing) == "" {
		return fmt.Errorf("HEAD is not reachable from any remote-tracking branch, it may hold unpublished commits")
	}

	return nil
}

// gitOutput runs `git -C dir <args...>` and returns its trimmed stdout.
func gitOutput(dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.Output()
	return string(out), err
}
