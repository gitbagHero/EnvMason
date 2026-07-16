package java

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

var fixtureTime = time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)

type runnerCall struct {
	path string
	args []string
	env  map[string]string
}

type fakeRunner struct {
	responses map[string]string
	failures  map[string]error
	calls     []runnerCall
}

func (r *fakeRunner) Run(_ context.Context, path string, args []string, _ string, environment map[string]string) (string, error) {
	key := commandKey(path, args)
	r.calls = append(r.calls, runnerCall{path: path, args: append([]string{}, args...), env: cloneMap(environment)})
	if err := r.failures[key]; err != nil {
		return "", err
	}
	if output, ok := r.responses[key]; ok {
		return output, nil
	}
	return "", errors.New("unexpected command")
}

func TestDiscoverSingleJDK(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	root := t.TempDir()
	jdkHome := filepath.Join(root, "jdks", "temurin-21", "Contents", "Home")
	writeRelease(t, jdkHome, "21.0.7", "Eclipse Adoptium", "aarch64")
	bin := filepath.Join(root, "bin")
	javaPath := writeExecutable(t, bin, "java")
	runner := newFakeRunner()
	request := fixtureRequest(root, []string{bin})
	request.JavaHome = jdkHome
	runner.responses[commandKey(request.JavaHomeTool, []string{"-X"})] = javaHomeXML(jdkHome, "21.0.7", "Eclipse Adoptium", "arm64")
	runner.responses[commandKey(javaPath, []string{"-XshowSettings:properties", "-version"})] = javaSettings("21.0.7", jdkHome, "Eclipse Adoptium", "aarch64")

	result, err := discover(context.Background(), request, dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.State != StateInstalled || len(result.JDKs) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Current.Version != "21.0.7" || result.Current.JDKID != result.JDKs[0].ID || result.Current.Home != "$HOME/jdks/temurin-21/Contents/Home" {
		t.Fatalf("current/JDK = %#v / %#v", result.Current, result.JDKs[0])
	}
	if result.Maven.State != StateNotInstalled || result.Gradle.State != StateNotInstalled {
		t.Fatalf("build tools = %#v / %#v", result.Maven, result.Gradle)
	}
	assertReadOnlyCalls(t, runner.calls)
}

func TestDiscoverMultipleJDKsBrokenJenvAndMavenMismatch(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	root := t.TempDir()
	systemHome := filepath.Join(root, "Library", "Java", "jdk-21", "Contents", "Home")
	writeRelease(t, systemHome, "21.0.7", "Microsoft", "arm64")
	prefix := filepath.Join(root, "homebrew")
	cellar := filepath.Join(prefix, "Cellar", "openjdk@25", "25.0.3")
	homebrewHome := filepath.Join(cellar, "libexec", "openjdk.jdk", "Contents", "Home")
	writeRelease(t, homebrewHome, "25.0.3", "Homebrew", "aarch64")
	opt := filepath.Join(prefix, "opt")
	if err := os.MkdirAll(opt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cellar, filepath.Join(opt, "openjdk@25")); err != nil {
		t.Fatal(err)
	}

	jenvRoot := filepath.Join(root, ".jenv")
	versions := filepath.Join(jenvRoot, "versions")
	if err := os.MkdirAll(versions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(systemHome, filepath.Join(versions, "21.0.7")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(homebrewHome, filepath.Join(versions, "25.0.3")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing-jdk"), filepath.Join(versions, "broken")); err != nil {
		t.Fatal(err)
	}
	writeText(t, filepath.Join(jenvRoot, "version"), "25.0.3\n")
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	writeText(t, filepath.Join(project, ".java-version"), "21.0.7\n")

	shimDirectory := filepath.Join(jenvRoot, "shims")
	javaPath := writeExecutable(t, shimDirectory, "java")
	toolBin := filepath.Join(root, "tools", "bin")
	mavenPath := writeExecutable(t, toolBin, "mvn")
	_ = writeExecutable(t, toolBin, "gradle")
	writeText(t, filepath.Join(filepath.Dir(toolBin), "lib", "gradle-core-9.1.0.jar"), "fixture")
	runner := newFakeRunner()
	request := fixtureRequest(project, []string{shimDirectory, toolBin})
	request.Home = root
	request.JavaHome = homebrewHome
	request.JenvRoot = jenvRoot
	request.HomebrewPrefixes = []string{prefix}
	runner.responses[commandKey(request.JavaHomeTool, []string{"-X"})] = javaHomeXML(systemHome, "21.0.7", "Microsoft", "arm64")
	runner.responses[commandKey(javaPath, []string{"-XshowSettings:properties", "-version"})] = javaSettings("25.0.3", homebrewHome, "Homebrew", "aarch64")
	runner.responses[commandKey(mavenPath, []string{"--version"})] = mavenOutput("3.9.16", "21.0.7", systemHome)

	result, err := discover(context.Background(), request, dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.JDKs) != 2 || result.Jenv.EffectiveVersion != "21.0.7" || result.Jenv.LocalVersionFile != "$HOME/project/.java-version" || !result.Jenv.Loaded {
		t.Fatalf("JDK/jenv = %#v / %#v", result.JDKs, result.Jenv)
	}
	if result.Maven.JavaVersion != "21.0.7" || result.Gradle.JavaVersion != "25.0.3" || result.Gradle.JavaHome != "$HOME/homebrew/Cellar/openjdk@25/25.0.3/libexec/openjdk.jdk/Contents/Home" {
		t.Fatalf("Maven/Gradle = %#v / %#v", result.Maven, result.Gradle)
	}
	assertFindingCode(t, result.Findings, "JENV_REGISTRATION_BROKEN")
	assertFindingCode(t, result.Findings, "JENV_RUNTIME_MISMATCH")
	assertFindingCode(t, result.Findings, "MAVEN_JAVA_MISMATCH")
	assertNoFindingCode(t, result.Findings, "GRADLE_JAVA_MISMATCH")
	assertReadOnlyCalls(t, runner.calls)
}

func TestNonexistentProjectDoesNotInventJenvLocalVersion(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	root := t.TempDir()
	jenvRoot := filepath.Join(root, ".jenv")
	if err := os.MkdirAll(jenvRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeText(t, filepath.Join(jenvRoot, "version"), "21\n")
	request := fixtureRequest(filepath.Join(root, "missing-project"), []string{filepath.Join(root, "empty")})
	request.Home = root
	request.JenvRoot = jenvRoot
	runner := newFakeRunner()
	runner.failures[commandKey(request.JavaHomeTool, []string{"-X"})] = errors.New("no registry")
	result, err := discover(context.Background(), request, dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.Jenv.LocalVersion != unknown || result.Jenv.LocalVersionFile != unknown || result.Jenv.EffectiveVersion != "21" {
		t.Fatalf("jenv = %#v", result.Jenv)
	}
}

func TestCommandFailuresAreIsolatedAndSanitized(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	const secret = "java-secret-must-not-leak"
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	javaPath := writeExecutable(t, bin, "java")
	mavenPath := writeExecutable(t, bin, "mvn")
	_ = writeExecutable(t, bin, "gradle")
	request := fixtureRequest(root, []string{bin})
	runner := newFakeRunner()
	runner.failures[commandKey(request.JavaHomeTool, []string{"-X"})] = errors.New(secret)
	runner.failures[commandKey(javaPath, []string{"-XshowSettings:properties", "-version"})] = errors.New(secret)
	runner.responses[commandKey(mavenPath, []string{"--version"})] = mavenOutput("3.9.16", "21.0.7", "/jdk/21")

	result, err := discover(context.Background(), request, dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), secret) {
		t.Fatal("command error leaked into result")
	}
	if result.Maven.State != StateInstalled || result.Gradle.State != StateUnknown {
		t.Fatalf("build tools = %#v / %#v", result.Maven, result.Gradle)
	}
	assertFindingCode(t, result.Findings, "JAVA_HOME_REGISTRY_QUERY_FAILED")
	assertFindingCode(t, result.Findings, "JAVA_RUNTIME_QUERY_FAILED")
	assertFindingCode(t, result.Findings, "GRADLE_METADATA_UNAVAILABLE")
}

func TestDiscoverRealJavaEnvironmentReadOnly(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("real Java acceptance requires macOS")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	jenvRoot := os.Getenv("JENV_ROOT")
	if jenvRoot == "" {
		jenvRoot = filepath.Join(home, ".jenv")
	}
	request := Request{
		PathDirectories: strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)), WorkingDirectory: workingDirectory,
		Home: home, JavaHome: os.Getenv("JAVA_HOME"), JenvRoot: jenvRoot, JenvShellVersion: os.Getenv("JENV_VERSION"),
		HomebrewPrefixes: []string{"/opt/homebrew", "/usr/local"}, GradleUserHome: os.Getenv("GRADLE_USER_HOME"),
		CollectedAt: time.Now().UTC(), ProcessArchitecture: inventory.Architecture(runtime.GOARCH),
	}
	before := ecosystemSnapshot(t, home, jenvRoot, request.GradleUserHome)
	result, err := Discover(context.Background(), request)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	after := ecosystemSnapshot(t, home, jenvRoot, request.GradleUserHome)
	if !slices.Equal(before, after) {
		t.Fatal("jenv, Maven, or Gradle metadata changed during discovery")
	}
	if result.State != StateInstalled || result.Current.State != StateInstalled || len(result.JDKs) < 1 {
		t.Fatalf("real result = %#v", result)
	}
	if result.Maven.State == StateInstalled && result.Maven.JavaVersion == unknown {
		t.Fatalf("Maven runtime missing: %#v", result.Maven)
	}
	t.Logf("real discovery: jdks=%d java=%s jenv=%s maven=%s gradle=%s findings=%d", len(result.JDKs), result.Current.Version, result.Jenv.EffectiveVersion, result.Maven.Version, result.Gradle.Version, len(result.Findings))
	for _, finding := range result.Findings {
		t.Logf("finding: %s (%s)", finding.Code, strings.Join(finding.Evidence, ", "))
	}
}

func fixtureRequest(workingDirectory string, path []string) Request {
	return Request{PathDirectories: path, WorkingDirectory: workingDirectory, Home: workingDirectory, JavaHomeTool: "/fixture/java_home", CollectedAt: fixtureTime, ProcessArchitecture: inventory.ArchitectureARM64}
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{responses: make(map[string]string), failures: make(map[string]error)}
}

func commandKey(path string, args []string) string { return path + "\x00" + strings.Join(args, "\x00") }

func javaHomeXML(home, version, vendor, architecture string) string {
	return fmt.Sprintf(`<?xml version="1.0"?><plist version="1.0"><array><dict><key>JVMArch</key><string>%s</string><key>JVMHomePath</key><string>%s</string><key>JVMName</key><string>OpenJDK %s</string><key>JVMVendor</key><string>%s</string><key>JVMVersion</key><string>%s</string></dict></array></plist>`, architecture, home, version, vendor, version)
}

func javaSettings(version, home, vendor, architecture string) string {
	return fmt.Sprintf("Property settings:\n    java.home = %s\n    java.version = %s\n    java.vendor = %s\n    os.arch = %s\nopenjdk version %q\n", home, version, vendor, architecture, version)
}

func mavenOutput(version, javaVersion, javaHome string) string {
	return fmt.Sprintf("Apache Maven %s (fixture)\nMaven home: /opt/maven\nJava version: %s, vendor: Fixture, runtime: %s\n", version, javaVersion, javaHome)
}

func writeRelease(t *testing.T, home, version, vendor, architecture string) {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	writeText(t, filepath.Join(home, "release"), fmt.Sprintf("JAVA_VERSION=%q\nIMPLEMENTOR=%q\nOS_ARCH=%q\n", version, vendor, architecture))
}

func writeExecutable(t *testing.T, directory, name string) string {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 99\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeText(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFindingCode(t *testing.T, findings []inventory.Finding, code string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code {
			return
		}
	}
	t.Fatalf("finding %q not found in %#v", code, findings)
}

func assertNoFindingCode(t *testing.T, findings []inventory.Finding, code string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code {
			t.Fatalf("unexpected finding %q in %#v", code, findings)
		}
	}
}

func assertReadOnlyCalls(t *testing.T, calls []runnerCall) {
	t.Helper()
	allowed := map[string]bool{
		strings.Join([]string{"-X"}, "\x00"):                                    true,
		strings.Join([]string{"-XshowSettings:properties", "-version"}, "\x00"): true,
		strings.Join([]string{"--version"}, "\x00"):                             true,
	}
	for _, call := range calls {
		if !allowed[strings.Join(call.args, "\x00")] {
			t.Fatalf("unexpected command args: %#v", call.args)
		}
		if call.env["MAVEN_SKIP_RC"] != "1" || call.env["LANG"] != "C" {
			t.Fatalf("unsafe environment = %#v", call.env)
		}
	}
}

func cloneMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func requirePOSIXFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Java/jenv fixture uses POSIX execute permissions and symlinks")
	}
}

func directorySnapshot(t *testing.T, root string) []string {
	t.Helper()
	result := []string{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr == nil {
			relative, _ := filepath.Rel(root, path)
			result = append(result, relative+"|"+info.ModTime().UTC().Format(time.RFC3339Nano)+"|"+info.Mode().String())
		}
		return nil
	})
	slices.Sort(result)
	return result
}

func ecosystemSnapshot(t *testing.T, home, jenvRoot, gradleUserHome string) []string {
	t.Helper()
	if gradleUserHome == "" {
		gradleUserHome = filepath.Join(home, ".gradle")
	}
	result := []string{}
	for label, root := range map[string]string{"jenv": jenvRoot, "maven": filepath.Join(home, ".m2"), "gradle": gradleUserHome} {
		for _, entry := range directorySnapshot(t, root) {
			result = append(result, label+"|"+entry)
		}
	}
	slices.Sort(result)
	return result
}
