package execution

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFileStoreNeverPersistsMockToken(t *testing.T) {
	t.Parallel()
	const token = "mock-token-must-not-reach-disk"
	exitCode := 0
	runner := &fakeRunner{result: ProcessResult{ExitCode: &exitCode, Stdout: CapturedOutput{Text: "token=" + token}, Stderr: CapturedOutput{Text: token}}}
	executor, request, _, _ := testHarness(t, runner)
	root := filepath.Join(t.TempDir(), "operations")
	executor.Store = FileStore{Root: root}
	request.SensitiveValues = []string{token}
	record, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token) || !strings.Contains(string(data), redactedValue) {
		t.Fatalf("persisted redaction failed: %s", data)
	}
}

func TestFileStoreWritesProtectedVersionedRecordAndLoadsLatestState(t *testing.T) {
	t.Parallel()
	executor, request, memory, _ := testHarness(t, nil)
	completed, err := executor.Execute(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "operations")
	store := FileStore{Root: root}
	if err := store.Save(memory.records[0]); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(completed); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(completed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StateCompleted || loaded.PlanID != request.Plan.ID {
		t.Fatalf("loaded = %#v", loaded)
	}
	data, err := os.ReadFile(filepath.Join(root, completed.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schema_version": "0.2.0"`) {
		t.Fatal("persisted record has no operation schema version")
	}
	if runtime.GOOS != "windows" {
		rootInfo, err := os.Stat(root)
		if err != nil {
			t.Fatal(err)
		}
		fileInfo, err := os.Stat(filepath.Join(root, completed.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if rootInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf("permissions = %o/%o", rootInfo.Mode().Perm(), fileInfo.Mode().Perm())
		}
	}
}

func TestFileStoreRejectsSymlinkDestinations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows CI")
	}
	t.Parallel()
	executor, request, memory, _ := testHarness(t, nil)
	if _, err := executor.Execute(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("do not replace"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, memory.records[0].ID+".json")
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}
	if err := (FileStore{Root: root}).Save(memory.records[0]); err == nil {
		t.Fatal("symlink destination was accepted")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "do not replace" {
		t.Fatalf("target changed: %q, %v", data, err)
	}
}

func TestFileStoreRejectsInvalidIdentityAndCorruptJSON(t *testing.T) {
	t.Parallel()
	store := FileStore{Root: t.TempDir()}
	if _, err := store.Load("../escape"); err == nil {
		t.Fatal("path traversal operation ID was accepted")
	}
	id := "op-00000000000000000000000000000002"
	if err := os.WriteFile(filepath.Join(store.Root, id+".json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(id); err == nil {
		t.Fatal("corrupt record was accepted")
	}
}

func TestDefaultHistoryDirectoryUsesNativeStateLocation(t *testing.T) {
	t.Parallel()
	path, err := DefaultHistoryDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if path == "" || !filepath.IsAbs(path) || filepath.Base(path) != "operations" {
		t.Fatalf("history path = %q", path)
	}
	if runtime.GOOS == "linux" && !strings.Contains(path, filepath.Join(".local", "state")) && os.Getenv("XDG_STATE_HOME") == "" {
		t.Fatalf("Linux history path = %q", path)
	}
}

func TestRecoverInterruptedRejectsTerminalRecord(t *testing.T) {
	t.Parallel()
	executor, request, _, _ := testHarness(t, nil)
	record, err := executor.Execute(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverInterrupted(record, testBaseTime.Add(3*time.Minute)); err == nil {
		t.Fatal("completed record was recoverable as interrupted")
	}
}
