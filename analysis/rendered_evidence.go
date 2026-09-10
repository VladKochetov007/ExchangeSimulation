package analysis

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type renderedEvidenceIdentity struct {
	Data struct {
		Sequence uint64 `json:"sequence"`
	} `json:"data"`
}

// digestRenderedEvidenceDirectory reproduces the renderer's deterministic
// digest over routed JSON records. It includes the route and local sequence,
// so a copied, reordered, or cross-routed rendered set cannot inherit a
// source attestation accidentally.
func digestRenderedEvidenceDirectory(dir string) (string, error) {
	venueRoot := filepath.Join(dir, "venues")
	var paths []string
	if err := filepath.WalkDir(venueRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".jsonl") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(paths)
	hasher := sha256.New()
	var scratch [8]byte
	for _, path := range paths {
		relative, err := filepath.Rel(venueRoot, path)
		if err != nil {
			return "", err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) < 2 || parts[0] == "" {
			return "", fmt.Errorf("analysis: rendered evidence path %q is not venue-qualified", relative)
		}
		venue := parts[0]
		route := strings.Join(parts[1:], "/")
		hasher.Write([]byte(venue))
		hasher.Write([]byte{0})
		hasher.Write([]byte(route))
		hasher.Write([]byte{0})
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		var previousSequence uint64
		for scanner.Scan() {
			raw := scanner.Bytes()
			var identity renderedEvidenceIdentity
			if err := json.Unmarshal(raw, &identity); err != nil {
				file.Close()
				return "", fmt.Errorf("analysis: decode rendered evidence %s: %w", relative, err)
			}
			if identity.Data.Sequence == 0 || identity.Data.Sequence <= previousSequence {
				file.Close()
				return "", fmt.Errorf("analysis: rendered evidence %s has non-increasing local sequence", relative)
			}
			previousSequence = identity.Data.Sequence
			binary.BigEndian.PutUint64(scratch[:], identity.Data.Sequence)
			hasher.Write(scratch[:])
			binary.BigEndian.PutUint64(scratch[:], uint64(len(raw)))
			hasher.Write(scratch[:])
			hasher.Write(raw)
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
