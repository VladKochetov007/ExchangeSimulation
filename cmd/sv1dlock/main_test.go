package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunPassesHeldDescriptorAndRejectsConcurrentInvocation(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "namespace.lock")
	readyPath := filepath.Join(root, "child.ready")
	if err := os.WriteFile(lockPath, []byte("retained-lock-marker"), 0o600); err != nil {
		t.Fatal(err)
	}

	childScript := "test \"$(readlink /proc/self/fd/3)\" = \"$1\" || exit 41; " +
		"flock -n 3 || exit 42; printf ready > \"$2\"; sleep 1"
	childArgs := []string{"-path", lockPath, "--", "/bin/sh", "-c", childScript, "child", lockPath, readyPath}
	firstDone := make(chan int, 1)
	go func() {
		firstDone <- run(childArgs, nil, io.Discard, io.Discard)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			firstStatus := <-firstDone
			t.Fatalf("locked child did not reach descriptor check, status %d", firstStatus)
		}
		time.Sleep(10 * time.Millisecond)
	}

	secondStatus := run([]string{"-path", lockPath, "--", "/bin/true"}, nil, io.Discard, io.Discard)
	if secondStatus == 0 {
		t.Fatal("concurrent locked invocation unexpectedly succeeded")
	}
	if firstStatus := <-firstDone; firstStatus != 0 {
		t.Fatalf("first locked invocation status = %d, want 0", firstStatus)
	}
	content, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "retained-lock-marker" {
		t.Fatalf("lock content = %q, helper or child truncated it", content)
	}
}

func TestRunRejectsFinalAndParentSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.lock")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	finalLink := filepath.Join(root, "final.lock")
	if err := os.Symlink(target, finalLink); err != nil {
		t.Fatal(err)
	}
	if status := run([]string{"-path", finalLink, "--", "/bin/true"}, nil, io.Discard, io.Discard); status == 0 {
		t.Fatal("command accepted a symlinked final lock path")
	}

	parentTarget := filepath.Join(root, "real-parent")
	if err := os.Mkdir(parentTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	parentLink := filepath.Join(root, "linked-parent")
	if err := os.Symlink(parentTarget, parentLink); err != nil {
		t.Fatal(err)
	}
	if status := run([]string{"-path", filepath.Join(parentLink, "lock"), "--", "/bin/true"}, nil, io.Discard, io.Discard); status == 0 {
		t.Fatal("command accepted a symlinked lock parent")
	}
}

func TestRunPropagatesChildExitStatus(t *testing.T) {
	var stderr bytes.Buffer
	status := run([]string{"-path", filepath.Join(t.TempDir(), "namespace.lock"), "--", "/bin/sh", "-c", "exit 17"}, nil, io.Discard, &stderr)
	if status != 17 {
		t.Fatalf("child exit status = %d, want 17 (stderr %q)", status, stderr.String())
	}
}
