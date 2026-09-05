package multivenue

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

// A run has to be able to say how it was compiled, not only which source it
// came from. The same revision and seed at GOAMD64=v3 produces a different
// execution stream hash from the default, with an identical event count, so a
// manifest that omits the toolchain cannot distinguish a failed reproduction
// from a semantic change.
func TestBuildInfoRecordsTheToolchain(t *testing.T) {
	build := currentBuild()
	if build.GoVersion == "" {
		t.Error("go_version is empty, so a run cannot say which compiler produced it")
	}
	if !strings.HasPrefix(build.GoVersion, "go") {
		t.Errorf("go_version %q does not look like a Go version", build.GoVersion)
	}
	if build.GOARCH != runtime.GOARCH {
		t.Errorf("goarch %q does not match the running binary's %q", build.GOARCH, runtime.GOARCH)
	}
	if build.GOOS != runtime.GOOS {
		t.Errorf("goos %q does not match the running binary's %q", build.GOOS, runtime.GOOS)
	}
}

// GOAMD64 is omitted when the toolchain default applies and present otherwise,
// so existing manifests built at the default are unchanged by this field.
func TestBuildInfoOmitsAnUnsetGOAMD64(t *testing.T) {
	raw, err := json.Marshal(BuildInfo{Revision: "abc", GoVersion: "go1.26.7", GOARCH: "amd64", GOOS: "linux"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "goamd64") {
		t.Errorf("an unset GOAMD64 was serialised: %s", raw)
	}
	raw, err = json.Marshal(BuildInfo{Revision: "abc", GOAMD64: "v3"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"goamd64":"v3"`) {
		t.Errorf("a set GOAMD64 was not serialised: %s", raw)
	}
}
