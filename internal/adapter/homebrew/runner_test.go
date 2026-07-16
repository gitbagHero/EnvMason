package homebrew

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestExecRunnerTrimsOutputAndSetsEnvironment(t *testing.T) {
	t.Parallel()

	output, err := runHelper(t, "output", map[string]string{"ENVMASON_TEST_VALUE": "fixture"})
	if err != nil || output.stdout != "fixture" {
		t.Fatalf("output = %#v, err = %v", output, err)
	}
}

func TestExecRunnerBoundsStdoutAndStderr(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"large-stdout", "large-stderr"} {
		if _, err := runHelper(t, mode, nil); err == nil {
			t.Fatalf("%s unexpectedly succeeded", mode)
		}
	}
}

func TestExecRunnerErrorDoesNotExposeStderr(t *testing.T) {
	t.Parallel()

	const secret = "runner-secret-must-not-leak"
	output, err := runHelper(t, "failure", nil, secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error = %v", err)
	}
	if output.stderr != secret {
		t.Fatalf("bounded stderr unavailable for internal classification: %q", output.stderr)
	}
}

func runHelper(t *testing.T, mode string, environment map[string]string, values ...string) (commandOutput, error) {
	t.Helper()
	args := []string{"-test.run=TestExecRunnerHelperProcess", "--", mode}
	args = append(args, values...)
	return (execRunner{}).Run(context.Background(), os.Args[0], args, environment)
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
	case "large-stdout":
		fmt.Print(strings.Repeat("x", maxStdout+2))
	case "large-stderr":
		fmt.Fprint(os.Stderr, strings.Repeat("x", maxStderr+2))
	case "failure":
		fmt.Fprint(os.Stderr, os.Args[separator+2])
		os.Exit(7)
	default:
		os.Exit(8)
	}
	os.Exit(0)
}
