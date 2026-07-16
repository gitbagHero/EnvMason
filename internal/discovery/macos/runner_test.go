package macos

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExecRunnerReturnsTrimmedOutput(t *testing.T) {
	t.Parallel()

	output, err := runHelper(t, execRunner{}, "output")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if output != "fixture output" {
		t.Fatalf("output = %q", output)
	}
}

func TestExecRunnerEnforcesTimeout(t *testing.T) {
	t.Parallel()

	_, err := runHelper(t, execRunner{timeout: time.Millisecond}, "sleep")
	if err == nil {
		t.Fatal("Run unexpectedly succeeded")
	}
}

func TestExecRunnerRejectsLargeOutput(t *testing.T) {
	t.Parallel()

	output, err := runHelper(t, execRunner{}, "large")
	if err == nil || err.Error() != "command output exceeded limit" {
		t.Fatalf("error = %v, output length = %d", err, len(output))
	}
}

func TestExecRunnerDoesNotExposeStderr(t *testing.T) {
	t.Parallel()

	const secret = "runner-secret-must-not-leak"
	_, err := runHelper(t, execRunner{}, "failure", secret)
	if err == nil {
		t.Fatal("Run unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("stderr secret leaked through runner error")
	}
}

func runHelper(t *testing.T, runner execRunner, mode string, values ...string) (string, error) {
	t.Helper()
	args := []string{"-test.run=TestExecRunnerHelperProcess", "--", mode}
	args = append(args, values...)
	return runner.Run(context.Background(), os.Args[0], args...)
}

func TestExecRunnerHelperProcess(t *testing.T) {
	separator := -1
	for index, value := range os.Args {
		if value == "--" {
			separator = index
			break
		}
	}
	if separator == -1 || separator+1 >= len(os.Args) {
		return
	}

	switch os.Args[separator+1] {
	case "output":
		fmt.Println("  fixture output  ")
	case "sleep":
		time.Sleep(200 * time.Millisecond)
	case "large":
		chunk := strings.Repeat("x", 4096)
		for range maxCommandOutput/len(chunk) + 2 {
			fmt.Print(chunk)
		}
	case "failure":
		fmt.Fprintln(os.Stderr, os.Args[separator+2])
		os.Exit(7)
	default:
		os.Exit(8)
	}
	os.Exit(0)
}
