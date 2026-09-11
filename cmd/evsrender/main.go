// Command evsrender reconstructs the routed JSONL evidence layout from a
// completed evstream_v3 run. It writes files below -out and emits only a
// compact attestation report on stdout.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	"exchange_sim/simulations/multivenue"
)

func main() {
	inputDir := flag.String("dir", "", "run directory containing events.evs")
	outputDir := flag.String("out", "", "empty directory receiving venues/<venue>/<route>.jsonl")
	flag.Parse()
	if *inputDir == "" || *outputDir == "" {
		fmt.Fprintln(os.Stderr, "evsrender: -dir and -out are required")
		os.Exit(2)
	}
	report, err := multivenue.RenderBinaryEvidence(*inputDir, *outputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evsrender: %v\n", err)
		os.Exit(1)
	}
	if err := writeRendererAttestation(*outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "evsrender: renderer attestation: %v\n", err)
		os.Exit(1)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evsrender: marshal report: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}

type rendererAttestation struct {
	SchemaVersion             int    `json:"schema_version"`
	Contract                  string `json:"contract"`
	RendererSHA256            string `json:"renderer_sha256"`
	RendererSourceRevision    string `json:"renderer_source_revision"`
	RendererSourceModified    bool   `json:"renderer_source_modified"`
	RendererGOOS              string `json:"renderer_goos"`
	RendererGOARCH            string `json:"renderer_goarch"`
	RendererGOAMD64           string `json:"renderer_goamd64"`
	RendererGoVersion         string `json:"renderer_go_version"`
	RendererTrimpath          bool   `json:"renderer_trimpath"`
	RendererCGOEnabled        string `json:"renderer_cgo_enabled"`
	RenderedAttestationSHA256 string `json:"rendered_attestation_sha256"`
}

func writeRendererAttestation(outputDir string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve executable symlinks: %w", err)
	}
	rendererSHA256, err := hashFile(executable)
	if err != nil {
		return fmt.Errorf("hash executable: %w", err)
	}
	build, err := currentBuild()
	if err != nil {
		return err
	}
	renderedAttestationPath := filepath.Join(outputDir, "rendered-binary-evidence-attestation.json")
	renderedAttestationSHA256, err := hashFile(renderedAttestationPath)
	if err != nil {
		return fmt.Errorf("hash rendered attestation: %w", err)
	}
	value := rendererAttestation{
		SchemaVersion: 1, Contract: "v2-r2-sv1d-renderer-attestation-v1",
		RendererSHA256: rendererSHA256, RendererSourceRevision: build.revision,
		RendererSourceModified: build.modified, RendererGOOS: build.goos,
		RendererGOARCH: build.goarch, RendererGOAMD64: build.goamd64,
		RendererGoVersion: build.goVersion, RendererTrimpath: build.trimpath,
		RendererCGOEnabled: build.cgoEnabled, RenderedAttestationSHA256: renderedAttestationSHA256,
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	path := filepath.Join(outputDir, "renderer-attestation.json")
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("refusing to overwrite %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary, err := os.CreateTemp(outputDir, ".renderer-attestation-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(append(raw, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryName, path); err != nil {
		return fmt.Errorf("publish without overwrite: %w", err)
	}
	return nil
}

type rendererBuild struct {
	revision   string
	modified   bool
	goos       string
	goarch     string
	goamd64    string
	goVersion  string
	trimpath   bool
	cgoEnabled string
}

func currentBuild() (rendererBuild, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return rendererBuild{}, fmt.Errorf("missing embedded build information")
	}
	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	revision, revisionOK := settings["vcs.revision"]
	modified, modifiedOK := settings["vcs.modified"]
	goos, goosOK := settings["GOOS"]
	goarch, goarchOK := settings["GOARCH"]
	goamd64, goamd64OK := settings["GOAMD64"]
	trimpath, trimpathOK := settings["-trimpath"]
	cgoEnabled, cgoOK := settings["CGO_ENABLED"]
	if !revisionOK || !modifiedOK || !goosOK || !goarchOK || !goamd64OK || !trimpathOK || !cgoOK || info.GoVersion == "" {
		return rendererBuild{}, fmt.Errorf("embedded build information is incomplete")
	}
	return rendererBuild{
		revision: revision, modified: modified == "true", goos: goos, goarch: goarch,
		goamd64: goamd64, goVersion: info.GoVersion, trimpath: trimpath == "true", cgoEnabled: cgoEnabled,
	}, nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
