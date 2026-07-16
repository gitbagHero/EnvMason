package executable

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

var collectedAt = time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)

type fakeFileInfo struct {
	name string
	mode fs.FileMode
}

func (i fakeFileInfo) Name() string       { return i.name }
func (i fakeFileInfo) Size() int64        { return 0 }
func (i fakeFileInfo) Mode() fs.FileMode  { return i.mode }
func (i fakeFileInfo) ModTime() time.Time { return collectedAt }
func (i fakeFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakeFileInfo) Sys() any           { return nil }

type fakeFileSystem struct {
	lstat      map[string]fakeFileInfo
	lstatErr   map[string]error
	stat       map[string]fakeFileInfo
	statErr    map[string]error
	resolved   map[string]string
	resolveErr map[string]error
}

func (f *fakeFileSystem) Lstat(path string) (fs.FileInfo, error) {
	if err := f.lstatErr[path]; err != nil {
		return nil, err
	}
	info, ok := f.lstat[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return info, nil
}

func (f *fakeFileSystem) Stat(path string) (fs.FileInfo, error) {
	if err := f.statErr[path]; err != nil {
		return nil, err
	}
	info, ok := f.stat[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return info, nil
}

func (f *fakeFileSystem) EvalSymlinks(path string) (string, error) {
	if err := f.resolveErr[path]; err != nil {
		return "", err
	}
	resolved, ok := f.resolved[path]
	if ok {
		return resolved, nil
	}
	if _, ok := f.lstat[path]; ok {
		return path, nil
	}
	return "", fs.ErrNotExist
}

type fakeArchitectureInspector struct {
	values map[string][]inventory.Architecture
	errors map[string]error
	paths  []string
}

func (i *fakeArchitectureInspector) Inspect(path string) ([]inventory.Architecture, error) {
	i.paths = append(i.paths, path)
	if err := i.errors[path]; err != nil {
		return nil, err
	}
	return i.values[path], nil
}

func TestDiscoverFindsAllCandidatesInPATHOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	firstDirectory := filepath.Join(root, "first")
	secondDirectory := filepath.Join(root, "second")
	first := filepath.Join(firstDirectory, "node")
	second := filepath.Join(secondDirectory, "node")
	files := newFakeFileSystem()
	files.addExecutable(first)
	files.addExecutable(second)
	architectures := newFakeArchitectureInspector()
	architectures.values[first] = []inventory.Architecture{inventory.ArchitectureARM64}
	architectures.values[second] = []inventory.Architecture{inventory.ArchitectureAMD64}

	result, err := discover(context.Background(), request("node", root, firstDirectory, secondDirectory), dependencies{files: files, architectures: architectures})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(result.Candidates))
	}
	if !result.Candidates[0].Effective || result.Candidates[1].Effective {
		t.Fatalf("effective flags = %v/%v", result.Candidates[0].Effective, result.Candidates[1].Effective)
	}
	if result.Candidates[0].DirectoryPosition != 0 || result.Candidates[1].DirectoryPosition != 1 {
		t.Fatalf("positions = %d/%d", result.Candidates[0].DirectoryPosition, result.Candidates[1].DirectoryPosition)
	}
	if !slices.Equal(result.Candidates[0].Architectures, []inventory.Architecture{inventory.ArchitectureARM64}) {
		t.Fatalf("first architectures = %#v", result.Candidates[0].Architectures)
	}
	assertFindingCode(t, result.Findings, "EXECUTABLE_PATH_SHADOWED")
}

func TestDiscoverMarksRepeatedPATHCandidate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	directory := filepath.Join(root, "bin")
	commandPath := filepath.Join(directory, "go")
	files := newFakeFileSystem()
	files.addExecutable(commandPath)

	result, err := discover(context.Background(), request("go", root, directory, directory), fixtureDependencies(files))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Candidates) != 2 || !result.Candidates[0].Duplicate || !result.Candidates[1].Duplicate {
		t.Fatalf("duplicate candidates = %#v", result.Candidates)
	}
	if !result.Candidates[0].Effective || result.Candidates[1].Effective {
		t.Fatalf("effective flags = %v/%v", result.Candidates[0].Effective, result.Candidates[1].Effective)
	}
}

func TestDiscoverHandlesBrokenAndLoopingSymlinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	broken := filepath.Join(root, "broken", "java")
	loop := filepath.Join(root, "loop", "java")
	valid := filepath.Join(root, "valid", "java")
	files := newFakeFileSystem()
	files.addSymlink(broken, "", fs.ErrNotExist)
	files.addSymlink(loop, "", errSymlinkLoop)
	files.addExecutable(valid)

	result, err := discover(context.Background(), request("java", root, filepath.Dir(broken), filepath.Dir(loop), filepath.Dir(valid)), fixtureDependencies(files))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Candidates) != 3 {
		t.Fatalf("candidates = %d, want 3", len(result.Candidates))
	}
	if result.Candidates[0].LinkState != LinkStateBroken || result.Candidates[1].LinkState != LinkStateLoop {
		t.Fatalf("link states = %q/%q", result.Candidates[0].LinkState, result.Candidates[1].LinkState)
	}
	if !result.Candidates[2].Effective {
		t.Fatal("valid candidate was not marked effective")
	}
	assertFindingCode(t, result.Findings, "EXECUTABLE_LINK_BROKEN")
	assertFindingCode(t, result.Findings, "EXECUTABLE_LINK_LOOP")
}

func TestDiscoverSupportsSpacesAndUnicode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	directory := filepath.Join(root, "开发 工具")
	command := "境匠 tool"
	commandPath := filepath.Join(directory, command)
	files := newFakeFileSystem()
	files.addExecutable(commandPath)

	result, err := discover(context.Background(), request(command, root, directory), fixtureDependencies(files))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	wantPath := filepath.Join("$HOME", "开发 工具", "境匠 tool")
	if len(result.Candidates) != 1 || result.Candidates[0].Path != wantPath {
		t.Fatalf("candidate = %#v", result.Candidates)
	}
}

func TestDiscoverPermissionFindingDoesNotStopLaterCandidates(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deniedDirectory := filepath.Join(root, "denied")
	validDirectory := filepath.Join(root, "valid")
	denied := filepath.Join(deniedDirectory, "python")
	valid := filepath.Join(validDirectory, "python")
	files := newFakeFileSystem()
	files.lstatErr[denied] = fs.ErrPermission
	files.addExecutable(valid)

	result, err := discover(context.Background(), request("python", root, deniedDirectory, validDirectory), fixtureDependencies(files))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Candidates) != 1 || !result.Candidates[0].Effective {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	assertFindingCode(t, result.Findings, "EXECUTABLE_PERMISSION_DENIED")
}

func TestDiscoverSkipsNonExecutableCandidateForEffectiveSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	firstDirectory := filepath.Join(root, "first")
	secondDirectory := filepath.Join(root, "second")
	first := filepath.Join(firstDirectory, "mvn")
	second := filepath.Join(secondDirectory, "mvn")
	files := newFakeFileSystem()
	files.lstat[first] = fakeFileInfo{name: "mvn", mode: 0o644}
	files.addExecutable(second)

	result, err := discover(context.Background(), request("mvn", root, firstDirectory, secondDirectory), fixtureDependencies(files))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.Candidates[0].Effective || !result.Candidates[1].Effective {
		t.Fatalf("effective flags = %v/%v", result.Candidates[0].Effective, result.Candidates[1].Effective)
	}
	assertFindingCode(t, result.Findings, "EXECUTABLE_NOT_EXECUTABLE")
}

func TestDiscoverResolvesSymlinkBeforeArchitectureInspection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	directory := filepath.Join(root, "bin")
	link := filepath.Join(directory, "node")
	target := filepath.Join(root, "versions", "node")
	files := newFakeFileSystem()
	files.addSymlink(link, target, nil)
	files.stat[target] = fakeFileInfo{name: "node", mode: 0o755}
	architectures := newFakeArchitectureInspector()
	architectures.values[target] = []inventory.Architecture{inventory.ArchitectureAMD64, inventory.ArchitectureARM64}

	result, err := discover(context.Background(), request("node", root, directory), dependencies{files: files, architectures: architectures})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	wantResolvedPath := filepath.Join("$HOME", "versions", "node")
	if result.Candidates[0].LinkState != LinkStateResolved || result.Candidates[0].ResolvedPath != wantResolvedPath {
		t.Fatalf("candidate = %#v", result.Candidates[0])
	}
	if !slices.Equal(architectures.paths, []string{target}) {
		t.Fatalf("inspected paths = %#v", architectures.paths)
	}
	if result.Candidates[0].AccessPath() != target {
		t.Fatalf("access path = %q, want %q", result.Candidates[0].AccessPath(), target)
	}
}

func TestDiscoverResolvesSymlinkedPATHDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	linkedDirectory := filepath.Join(root, "linked-bin")
	encountered := filepath.Join(linkedDirectory, "go")
	target := filepath.Join(root, "real-bin", "go")
	files := newFakeFileSystem()
	files.lstat[encountered] = fakeFileInfo{name: "go", mode: 0o755}
	files.resolved[encountered] = target
	files.stat[target] = fakeFileInfo{name: "go", mode: 0o755}

	result, err := discover(context.Background(), request("go", root, linkedDirectory), fixtureDependencies(files))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	wantResolvedPath := filepath.Join("$HOME", "real-bin", "go")
	if result.Candidates[0].LinkState != LinkStateResolved || result.Candidates[0].ResolvedPath != wantResolvedPath {
		t.Fatalf("candidate = %#v", result.Candidates[0])
	}
}

func TestDiscoverReportsArchitecturePermissionFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	directory := filepath.Join(root, "bin")
	commandPath := filepath.Join(directory, "node")
	files := newFakeFileSystem()
	files.addExecutable(commandPath)
	architectures := newFakeArchitectureInspector()
	architectures.errors[commandPath] = fs.ErrPermission

	result, err := discover(context.Background(), request("node", root, directory), dependencies{files: files, architectures: architectures})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Candidates) != 1 || !result.Candidates[0].Effective {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	assertFindingCode(t, result.Findings, "EXECUTABLE_ARCHITECTURE_PERMISSION_DENIED")
}

func TestDiscoverRejectsUnsafeCommandNames(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"", ".", "..", "../node", "/usr/bin/node", `dir\\node`, "bad\x00name", "bad\nname"} {
		command := command
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			_, err := discover(context.Background(), request(command, t.TempDir()), fixtureDependencies(newFakeFileSystem()))
			if err == nil {
				t.Fatal("discover unexpectedly accepted command")
			}
		})
	}
}

func TestDiscoverRequiresDeterministicContext(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	base := request("node", root, root)
	for name, mutate := range map[string]func(*Request){
		"missing working directory":  func(value *Request) { value.WorkingDirectory = "" },
		"relative working directory": func(value *Request) { value.WorkingDirectory = "relative" },
		"missing collection time":    func(value *Request) { value.CollectedAt = time.Time{} },
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			value := base
			mutate(&value)
			if _, err := discover(context.Background(), value, fixtureDependencies(newFakeFileSystem())); err == nil {
				t.Fatal("discover unexpectedly accepted an implicit input")
			}
		})
	}
}

func TestDiscoverHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := discover(ctx, request("node", t.TempDir(), t.TempDir()), fixtureDependencies(newFakeFileSystem()))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestDiscoverEmptyPATHEntryUsesWorkingDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	commandPath := filepath.Join(root, "node")
	files := newFakeFileSystem()
	files.addExecutable(commandPath)

	result, err := discover(context.Background(), request("node", root, ""), fixtureDependencies(files))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Candidates) != 1 || !result.Candidates[0].Effective {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	assertFindingCode(t, result.Findings, "EMPTY_PATH_ENTRY")
}

func TestDiscoverRealFilesystemDoesNotExecuteCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable mode integration")
	}

	root := t.TempDir()
	directory := filepath.Join(root, "bin with 空格")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	marker := filepath.Join(root, "executed")
	commandPath := filepath.Join(directory, "fixture")
	script := []byte("#!/bin/sh\ntouch \"" + marker + "\"\n")
	if err := os.WriteFile(commandPath, script, 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := Discover(context.Background(), request("fixture", root, directory))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(result.Candidates) != 1 || !result.Candidates[0].Effective {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	if _, err := os.Stat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("candidate appears to have executed: %v", err)
	}
}

func TestDiscoverRealFilesystemSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic-link integration requires no Windows developer-mode privilege")
	}

	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	brokenDirectory := filepath.Join(root, "broken")
	loopDirectory := filepath.Join(root, "loop")
	validDirectory := filepath.Join(root, "valid")
	for _, directory := range []string{brokenDirectory, loopDirectory, validDirectory} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", directory, err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(brokenDirectory, "fixture")); err != nil {
		t.Fatalf("broken symlink: %v", err)
	}
	loopCommand := filepath.Join(loopDirectory, "fixture")
	loopOther := filepath.Join(loopDirectory, "other")
	if err := os.Symlink(loopOther, loopCommand); err != nil {
		t.Fatalf("loop command symlink: %v", err)
	}
	if err := os.Symlink(loopCommand, loopOther); err != nil {
		t.Fatalf("loop other symlink: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(validDirectory, "fixture")); err != nil {
		t.Fatalf("valid symlink: %v", err)
	}

	result, err := Discover(context.Background(), request("fixture", root, brokenDirectory, loopDirectory, validDirectory))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(result.Candidates) != 3 {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	if result.Candidates[0].LinkState != LinkStateBroken || result.Candidates[1].LinkState != LinkStateLoop || !result.Candidates[2].Effective {
		t.Fatalf("symlink candidates = %#v", result.Candidates)
	}
}

func TestDiscoverCurrentGoCommandOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("real PATH acceptance requires macOS")
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	result, err := Discover(context.Background(), Request{
		Command:          "go",
		Directories:      filepath.SplitList(os.Getenv("PATH")),
		WorkingDirectory: workingDirectory,
		Home:             home,
		CollectedAt:      collectedAt,
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, candidate := range result.Candidates {
		if candidate.Effective {
			if slices.Equal(candidate.Architectures, []inventory.Architecture{inventory.ArchitectureUnknown}) {
				t.Fatalf("effective Go candidate architecture is unknown: %#v", candidate)
			}
			return
		}
	}
	t.Fatalf("no effective Go candidate found: %#v", result)
}

func request(command, root string, directories ...string) Request {
	return Request{
		Command:          command,
		Directories:      directories,
		WorkingDirectory: root,
		Home:             root,
		CollectedAt:      collectedAt,
	}
}

func newFakeFileSystem() *fakeFileSystem {
	return &fakeFileSystem{
		lstat: make(map[string]fakeFileInfo), lstatErr: make(map[string]error),
		stat: make(map[string]fakeFileInfo), statErr: make(map[string]error),
		resolved: make(map[string]string), resolveErr: make(map[string]error),
	}
}

func (f *fakeFileSystem) addExecutable(path string) {
	f.lstat[path] = fakeFileInfo{name: filepath.Base(path), mode: 0o755}
}

func (f *fakeFileSystem) addSymlink(path, target string, err error) {
	f.lstat[path] = fakeFileInfo{name: filepath.Base(path), mode: fs.ModeSymlink | 0o777}
	if err != nil {
		f.resolveErr[path] = err
		return
	}
	f.resolved[path] = target
}

func newFakeArchitectureInspector() *fakeArchitectureInspector {
	return &fakeArchitectureInspector{values: make(map[string][]inventory.Architecture), errors: make(map[string]error)}
}

func fixtureDependencies(files *fakeFileSystem) dependencies {
	return dependencies{files: files, architectures: newFakeArchitectureInspector()}
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
