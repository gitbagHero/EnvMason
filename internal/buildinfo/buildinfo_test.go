package buildinfo

import (
	"runtime"
	"testing"
)

func TestCurrent(t *testing.T) {
	info := Current()

	if info.Version == "" {
		t.Fatal("Version must not be empty")
	}
	if info.Commit == "" {
		t.Fatal("Commit must not be empty")
	}
	if info.BuildTime == "" {
		t.Fatal("BuildTime must not be empty")
	}
	if info.GoVersion != runtime.Version() {
		t.Fatalf("GoVersion = %q, want %q", info.GoVersion, runtime.Version())
	}
	wantTarget := runtime.GOOS + "/" + runtime.GOARCH
	if info.Target != wantTarget {
		t.Fatalf("Target = %q, want %q", info.Target, wantTarget)
	}
}
