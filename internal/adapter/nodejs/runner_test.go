package nodejs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExecRunnerTrimsOutputAndAppliesEnvironment(t *testing.T) {
	t.Parallel()
	output, err := runHelper(t, execRunner{}, "output", map[string]string{"ENVMASON_TEST_VALUE": "fixture"})
	if err != nil || output != "fixture" {
		t.Fatalf("output = %q, err = %v", output, err)
	}
}

func TestExecRunnerEnforcesTimeoutAndOutputBounds(t *testing.T) {
	t.Parallel()
	if _, err := runHelper(t, execRunner{timeout: time.Millisecond}, "sleep", nil); err == nil {
		t.Fatal("timeout unexpectedly succeeded")
	}
	for _, mode := range []string{"large-stdout", "large-stderr"} {
		if _, err := runHelper(t, execRunner{outputLimit: 1024}, mode, nil); err == nil {
			t.Fatalf("%s unexpectedly succeeded", mode)
		}
	}
}

func TestExecRunnerDoesNotExposeStderr(t *testing.T) {
	t.Parallel()
	const secret = "runner-secret-must-not-leak"
	_, err := runHelper(t, execRunner{}, "failure", nil, secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error = %v", err)
	}
}

func TestControlledEnvironmentDoesNotInheritNodeHooksOrSecrets(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "--require=/secret/hook.js")
	t.Setenv("NPM_TOKEN", "must-not-be-inherited")
	values := make(map[string]string)
	for _, entry := range controlledEnvironment(map[string]string{"PATH": "/safe/bin"}) {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	if values["PATH"] != "/safe/bin" {
		t.Fatalf("PATH = %q", values["PATH"])
	}
	if _, ok := values["NODE_OPTIONS"]; ok {
		t.Fatal("NODE_OPTIONS was inherited")
	}
	if _, ok := values["NPM_TOKEN"]; ok {
		t.Fatal("NPM_TOKEN was inherited")
	}
}

func runHelper(t *testing.T, runner execRunner, mode string, environment map[string]string, values ...string) (string, error) {
	t.Helper()
	args := []string{"-test.run=TestExecRunnerHelperProcess", "--", mode}
	args = append(args, values...)
	return runner.Run(context.Background(), os.Args[0], args, t.TempDir(), environment)
}

func TestExecRunnerHelperProcess(t *testing.T) {
	separator := -1
	for index, value := range os.Args {
		if value == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	switch os.Args[separator+1] {
	case "output":
		fmt.Printf("  %s  \n", os.Getenv("ENVMASON_TEST_VALUE"))
	case "sleep":
		time.Sleep(200 * time.Millisecond)
	case "large-stdout":
		fmt.Print(strings.Repeat("x", 4096))
	case "large-stderr":
		fmt.Fprint(os.Stderr, strings.Repeat("x", 4096))
	case "failure":
		fmt.Fprint(os.Stderr, os.Args[separator+2])
		os.Exit(7)
	default:
		os.Exit(8)
	}
	os.Exit(0)
}
