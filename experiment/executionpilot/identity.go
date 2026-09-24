package executionpilot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
)

func RuntimeIdentity(repositoryDir, analyzerBinary, evidenceSchema string) (Identity, error) {
	binaryPath, err := os.Executable()
	if err != nil {
		return Identity{}, fmt.Errorf("execution pilot: resolve simulator binary: %w", err)
	}
	return ToolIdentity(repositoryDir, binaryPath, analyzerBinary, evidenceSchema)
}

// ToolIdentity binds both binaries to a clean source checkout. The caller must
// additionally verify that the binary it is currently executing is the named
// simulator or analyzer, as appropriate for its command.
func ToolIdentity(repositoryDir, simulatorBinary, analyzerBinary, evidenceSchema string) (Identity, error) {
	build, ok := debug.ReadBuildInfo()
	if !ok || build.GoVersion == "" {
		return Identity{}, fmt.Errorf("execution pilot: missing simulator build metadata")
	}
	settings := make(map[string]string, len(build.Settings))
	for _, setting := range build.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["vcs"] != "git" || settings["vcs.modified"] != "false" ||
		!hexDigest(settings["vcs.revision"], 20) {
		return Identity{}, fmt.Errorf("execution pilot: simulator was not built from a clean pinned Git revision")
	}
	commit, err := gitValue(repositoryDir, "rev-parse", "HEAD")
	if err != nil {
		return Identity{}, err
	}
	if commit != settings["vcs.revision"] {
		return Identity{}, fmt.Errorf("execution pilot: simulator revision differs from checkout HEAD")
	}
	status, err := gitValue(repositoryDir, "status", "--porcelain")
	if err != nil || status != "" {
		return Identity{}, fmt.Errorf("execution pilot: source checkout is not clean: %w", err)
	}
	tree, err := gitValue(repositoryDir, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return Identity{}, err
	}
	simulatorDigest, err := fileSHA256(simulatorBinary)
	if err != nil {
		return Identity{}, fmt.Errorf("execution pilot: hash simulator binary: %w", err)
	}
	analyzerDigest, err := fileSHA256(analyzerBinary)
	if err != nil {
		return Identity{}, fmt.Errorf("execution pilot: hash analyzer binary: %w", err)
	}
	identity := Identity{
		SourceCommit: commit, SourceTree: tree,
		SimulatorSHA256: simulatorDigest, AnalyzerSHA256: analyzerDigest,
		Toolchain: build.GoVersion, EvidenceSchemaID: evidenceSchema,
	}
	if err := ValidateIdentityForSchema(identity, evidenceSchema); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

func gitValue(dir string, arguments ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", dir}, arguments...)...)
	result, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("execution pilot: git %s: %w", strings.Join(arguments, " "), err)
	}
	return strings.TrimSpace(string(result)), nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
