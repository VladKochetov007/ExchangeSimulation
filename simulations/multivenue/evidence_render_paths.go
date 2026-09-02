package multivenue

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func canonicalRenderPath(path string, allowMissing bool) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("multivenue: resolve render path %q: %w", path, err)
	}
	if err := rejectRenderPathSymlinks(absolutePath); err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(absolutePath)
	if err == nil {
		return filepath.Clean(resolvedPath), nil
	}
	if !allowMissing || !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("multivenue: canonicalize render path %q: %w", path, err)
	}

	missingComponents := make([]string, 0, 2)
	existingAncestor := absolutePath
	for {
		_, statErr := os.Lstat(existingAncestor)
		if statErr == nil {
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("multivenue: inspect render path %q: %w", path, statErr)
		}
		parent := filepath.Dir(existingAncestor)
		if parent == existingAncestor {
			return "", fmt.Errorf("multivenue: render path %q has no existing ancestor", path)
		}
		missingComponents = append(missingComponents, filepath.Base(existingAncestor))
		existingAncestor = parent
	}
	resolvedAncestor, err := filepath.EvalSymlinks(existingAncestor)
	if err != nil {
		return "", fmt.Errorf("multivenue: canonicalize render path ancestor %q: %w", existingAncestor, err)
	}
	for index := len(missingComponents) - 1; index >= 0; index-- {
		resolvedAncestor = filepath.Join(resolvedAncestor, missingComponents[index])
	}
	return filepath.Clean(resolvedAncestor), nil
}

func rejectRenderPathSymlinks(path string) error {
	currentPath := filepath.Clean(path)
	for {
		info, err := os.Lstat(currentPath)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("multivenue: render path contains symlink %q", currentPath)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("multivenue: inspect render path component %q: %w", currentPath, err)
		}
		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			return nil
		}
		currentPath = parent
	}
}

func validateRenderOutputDirectory(path string) error {
	if err := rejectRenderPathSymlinks(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("multivenue: inspect render output: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("multivenue: render output is a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("multivenue: render output is not a directory")
	}
	return nil
}

func readRenderRegularFile(path, description string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("multivenue: %s: %w", description, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("multivenue: %s is not a regular file", description)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("multivenue: %s: %w", description, err)
	}
	return raw, nil
}

func openRenderRegularFile(path, description string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("multivenue: %s: %w", description, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("multivenue: %s is not a regular file", description)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("multivenue: %s: %w", description, err)
	}
	return file, nil
}
