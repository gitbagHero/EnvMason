package java

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExecRunnerCombinesOutputAndUsesControlledEnvironment(t *testing.T) {
	t.Parallel()
	output, err := runHelper(t, execRunner{}, "output", map[string]string{"ENVMASON_TEST_VALUE": "fixture"})
	if err != nil || !strings.Contains(output, "fixture stdout") || !strings.Contains(output, "fixture stderr") {
		t.Fatalf("output = %q, err = %v", output, err)
	}
}

func TestExecRunnerEnforcesTimeoutAndOutputLimit(t *testing.T) {
	t.Parallel()
	if _, err := runHelper(t, execRunner{timeout: time.Millisecond}, "sleep", nil); err == nil {
		t.Fatal("timeout unexpectedly succeeded")
	}
	if _, err := runHelper(t, execRunner{outputLimit: 1024}, "large", nil); err == nil {
		t.Fatal("large output unexpectedly succeeded")
	}
}

func TestControlledEnvironmentDropsJavaHooksAndSecrets(t *testing.T) {
	t.Setenv("JAVA_TOOL_OPTIONS", "-javaagent:/secret.jar")
	t.Setenv("MAVEN_OPTS", "secret")
	values := make(map[string]string)
	for _, entry := range controlledEnvironment(map[string]string{"PATH": "/safe/bin"}) {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	if values["PATH"] != "/safe/bin" {
		t.Fatalf("PATH = %q", values["PATH"])
	}
	if _, ok := values["JAVA_TOOL_OPTIONS"]; ok {
		t.Fatal("JAVA_TOOL_OPTIONS was inherited")
	}
	if _, ok := values["MAVEN_OPTS"]; ok {
		t.Fatal("MAVEN_OPTS was inherited")
	}
}

func runHelper(t *testing.T, runner execRunner, mode string, environment map[string]string) (string, error) {
	t.Helper()
	return runner.Run(context.Background(), os.Args[0], []string{"-test.run=TestExecRunnerHelperProcess", "--", mode}, t.TempDir(), environment)
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
		fmt.Printf("%s stdout\n", os.Getenv("ENVMASON_TEST_VALUE"))
		fmt.Fprintf(os.Stderr, "%s stderr\n", os.Getenv("ENVMASON_TEST_VALUE"))
	case "sleep":
		time.Sleep(200 * time.Millisecond)
	case "large":
		fmt.Print(strings.Repeat("x", 4096))
	default:
		os.Exit(8)
	}
	os.Exit(0)
}
