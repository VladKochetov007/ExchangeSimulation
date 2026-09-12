//go:build linux

package sv1dlock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireDoesNotTruncateAndEnforcesExclusiveOwnership(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "namespace.lock")
	if err := os.WriteFile(path, []byte("retained-lock-marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "retained-lock-marker" {
		t.Fatalf("lock content = %q, lock acquisition truncated an existing file", content)
	}
	if _, err := Acquire(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second acquisition error = %v, want ErrLocked", err)
	}
}

func TestAcquireRejectsFinalAndParentSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.lock")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	finalLink := filepath.Join(root, "final.lock")
	if err := os.Symlink(target, finalLink); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(finalLink); err == nil {
		t.Fatal("final lock symlink was followed")
	}
	parentTarget := filepath.Join(root, "real-parent")
	if err := os.Mkdir(parentTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	parentLink := filepath.Join(root, "linked-parent")
	if err := os.Symlink(parentTarget, parentLink); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(filepath.Join(parentLink, "lock")); err == nil {
		t.Fatal("symlinked lock parent was followed")
	}
}
