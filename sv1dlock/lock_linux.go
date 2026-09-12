//go:build linux

// Package sv1dlock provides a descriptor-bound, non-destructive namespace
// lock for the SV1D scientific runners. It is intentionally separate from the
// runners so the shell adapters remain orchestration code and the path safety
// policy is reusable and testable.
package sv1dlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var ErrLocked = errors.New("sv1dlock: namespace is already locked")

// Lock owns the opened lock inode. Closing it releases the advisory lock;
// the lock file itself is intentionally retained so a completed run never
// needs to delete a path that another process could have replaced.
type Lock struct {
	file *os.File
}

// Acquire opens path relative to descriptor-validated parent directories and
// takes a non-blocking exclusive flock. Existing files are opened without
// truncation, and a final symlink or a symlinked parent is rejected by the
// kernel rather than only by a prior path inspection.
func Acquire(path string) (*Lock, error) {
	cleanPath, err := cleanLockPath(path)
	if err != nil {
		return nil, err
	}
	parentFD, err := openDirectoryPath(filepath.Dir(cleanPath))
	if err != nil {
		return nil, err
	}
	defer syscall.Close(parentFD)

	fd, err := syscall.Openat(parentFD, filepath.Base(cleanPath), syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", cleanPath, err)
	}
	file := os.NewFile(uintptr(fd), cleanPath)
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat lock %s: %w", cleanPath, err)
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		_ = file.Close()
		return nil, fmt.Errorf("lock %s is not a regular file", cleanPath)
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("%w: %s", ErrLocked, cleanPath)
		}
		return nil, fmt.Errorf("lock %s: %w", cleanPath, err)
	}
	return &Lock{file: file}, nil
}

// File returns the opened descriptor for use as an exec.Cmd ExtraFile. The
// caller must keep the Lock alive until the child command exits.
func (l *Lock) File() *os.File {
	if l == nil {
		return nil
	}
	return l.file
}

// Close releases the namespace lock.
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

func cleanLockPath(path string) (string, error) {
	if path == "" || strings.ContainsRune(path, '\x00') {
		return "", errors.New("lock path is empty or contains NUL")
	}
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) || cleanPath == string(filepath.Separator) || filepath.Base(cleanPath) == "." || filepath.Base(cleanPath) == ".." {
		return "", fmt.Errorf("lock path must be a clean non-root absolute path: %q", path)
	}
	return cleanPath, nil
}

func openDirectoryPath(path string) (int, error) {
	if path == string(filepath.Separator) {
		return syscall.Open(string(filepath.Separator), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, fmt.Errorf("directory path must be clean absolute: %q", path)
	}
	currentFD, err := syscall.Open(string(filepath.Separator), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return -1, fmt.Errorf("open lock root: %w", err)
	}
	for _, component := range strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" || component == "." || component == ".." {
			_ = syscall.Close(currentFD)
			return -1, fmt.Errorf("invalid lock parent component %q", component)
		}
		nextFD, openErr := syscall.Openat(currentFD, component, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if openErr != nil {
			_ = syscall.Close(currentFD)
			return -1, fmt.Errorf("open lock parent %s: %w", component, openErr)
		}
		_ = syscall.Close(currentFD)
		currentFD = nextFD
	}
	return currentFD, nil
}
