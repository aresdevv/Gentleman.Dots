package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const testRemoteSubstr = "gentleman-programming/gentleman.dots"

// runGit runs `git <args...>` in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

// newCloneMatchingRemote creates a bare "remote" repo and a clone of it whose
// origin URL is rewritten to contain the expected substring, simulating the
// installer's own clone of Gentleman.Dots.
func newCloneMatchingRemote(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	remoteDir := filepath.Join(base, "remote.git")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, remoteDir, "init", "--bare", "-b", "main")

	seedDir := filepath.Join(base, "seed")
	if err := os.MkdirAll(seedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, seedDir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(seedDir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seedDir, "add", ".")
	runGit(t, seedDir, "commit", "-m", "seed")
	runGit(t, seedDir, "remote", "add", "origin", remoteDir)
	runGit(t, seedDir, "push", "origin", "main")

	cloneDir := filepath.Join(base, "Gentleman.Dots")
	runGit(t, base, "clone", remoteDir, cloneDir)
	runGit(t, cloneDir, "remote", "set-url", "origin", "https://example.com/Gentleman-Programming/Gentleman.Dots.git")

	return cloneDir
}

func TestSafeRemoveClone(t *testing.T) {
	t.Run("does nothing when the path does not exist", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		if err := SafeRemoveClone(missing, testRemoteSubstr); err != nil {
			t.Fatalf("expected nil error for missing path, got %v", err)
		}
	})

	t.Run("refuses to delete a directory that is not a git repository", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "Gentleman.Dots")
		if err := os.MkdirAll(filepath.Join(dir, "unrelated-work"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "unrelated-work", "notes.txt"), []byte("do not delete"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := SafeRemoveClone(dir, testRemoteSubstr); err == nil {
			t.Fatal("expected an error, got nil")
		}

		if _, err := os.Stat(filepath.Join(dir, "unrelated-work", "notes.txt")); err != nil {
			t.Fatalf("directory should have been left intact: %v", err)
		}
	})

	t.Run("refuses to delete a git repo whose origin does not match", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "Gentleman.Dots")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		runGit(t, dir, "init", "-b", "main")
		if err := os.WriteFile(filepath.Join(dir, "work.txt"), []byte("local work"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", "unrelated local work")
		runGit(t, dir, "remote", "add", "origin", "https://example.com/someone-else/unrelated-repo.git")

		if err := SafeRemoveClone(dir, testRemoteSubstr); err == nil {
			t.Fatal("expected an error for a mismatched origin remote, got nil")
		}

		if _, err := os.Stat(filepath.Join(dir, "work.txt")); err != nil {
			t.Fatalf("unrelated repo should have been left intact: %v", err)
		}
	})

	t.Run("refuses to delete a matching clone with a dirty working tree", func(t *testing.T) {
		dir := newCloneMatchingRemote(t)
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("locally edited"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := SafeRemoveClone(dir, testRemoteSubstr); err == nil {
			t.Fatal("expected an error for a dirty working tree, got nil")
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("dirty clone should have been left intact: %v", err)
		}
	})

	t.Run("refuses to delete a matching clone with commits unpublished to any remote", func(t *testing.T) {
		dir := newCloneMatchingRemote(t)
		if err := os.WriteFile(filepath.Join(dir, "local-only.txt"), []byte("unpublished work"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", "unpublished commit")

		if err := SafeRemoveClone(dir, testRemoteSubstr); err == nil {
			t.Fatal("expected an error for HEAD not reachable from a remote-tracking branch, got nil")
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("clone with unpublished commits should have been left intact: %v", err)
		}
	})

	t.Run("deletes a clean matching clone", func(t *testing.T) {
		dir := newCloneMatchingRemote(t)

		if err := SafeRemoveClone(dir, testRemoteSubstr); err != nil {
			t.Fatalf("expected the clean matching clone to be removed, got error: %v", err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("expected directory to be removed, stat err = %v", err)
		}
	})
}
