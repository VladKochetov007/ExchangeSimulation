package exchange_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTrackedFilesAvoidSystemTempPaths(t *testing.T) {
	repositoryRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	tracked, err := exec.Command("git", "-C", repositoryRoot, "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("list tracked files: %v", err)
	}
	tempPath := []byte("/" + "tmp")
	for _, name := range bytes.Split(tracked, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		path := filepath.Join(repositoryRoot, string(name))
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read tracked file %s: %v", name, err)
		}
		if bytes.Contains(contents, tempPath) {
			t.Errorf("tracked file %s embeds a system temporary path; use a configured or repository-relative output path", name)
		}
	}
}
