package crossvenue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateResultPath keeps a new result outside immutable run and rendered
// evidence trees. The final writer must still use O_EXCL against races.
func ValidateResultPath(rawDir, renderedDir, resultPath string) error {
	if rawDir == "" || renderedDir == "" || resultPath == "" {
		return fmt.Errorf("ME-005 result path: missing input")
	}
	rawAbs, err := filepath.Abs(rawDir)
	if err != nil {
		return err
	}
	rawReal, err := filepath.EvalSymlinks(rawAbs)
	if err != nil {
		return fmt.Errorf("ME-005 result path: resolve raw run: %w", err)
	}
	renderedReal, err := crossVenueProspectivePath(renderedDir)
	if err != nil {
		return err
	}
	resultReal, err := crossVenueProspectivePath(resultPath)
	if err != nil {
		return err
	}
	if crossVenueWithin(rawReal, resultReal) || crossVenueWithin(renderedReal, resultReal) {
		return fmt.Errorf("ME-005 result path: result would modify an evidence tree")
	}
	if _, err := os.Lstat(resultPath); err == nil {
		return fmt.Errorf("ME-005 result path: result already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func crossVenueProspectivePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("ME-005 result path: resolve parent of %s: %w", path, err)
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func crossVenueWithin(base, candidate string) bool {
	relative, err := filepath.Rel(base, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
