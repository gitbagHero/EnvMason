package projectscan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)

func TestNoRootsDoesNotScanWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, ".nvmrc"), "20\n")
	result := Scan(context.Background(), Request{WorkingDirectory: root, Home: root, CollectedAt: testTime})
	if len(result.Projects) != 0 || len(result.Issues) != 0 {
		t.Fatalf("implicit scan result = %#v", result)
	}
}

func TestScansAllStaticFormatsAndFindsProvableConflicts(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "workspace", "app")
	writeFixture(t, filepath.Join(root, ".nvmrc"), "20\n")
	writeFixture(t, filepath.Join(root, ".node-version"), "v20.0.0\n")
	writeFixture(t, filepath.Join(root, "package.json"), `{"name":"app","engines":{"node":">=22 <23"}}`)
	writeFixture(t, filepath.Join(root, ".java-version"), "21\n")
	writeFixture(t, filepath.Join(root, ".tool-versions"), "nodejs 20.0.0\njava 21\npython 3.12\n")
	writeFixture(t, filepath.Join(root, "pom.xml"), `<project><properties><java.version>17</java.version><maven.compiler.release>17</maven.compiler.release></properties></project>`)
	writeFixture(t, filepath.Join(root, "build.gradle.kts"), `java { toolchain { languageVersion.set(JavaLanguageVersion.of(21)) } }`)

	result := Scan(context.Background(), Request{Roots: []string{root}, Home: home, CollectedAt: testTime})
	if len(result.Projects) != 1 {
		t.Fatalf("projects = %#v", result.Projects)
	}
	project := result.Projects[0]
	if project.Root != "$HOME/workspace/app" || len(project.References) != 9 {
		t.Fatalf("project = %#v", project)
	}
	if countIssue(result.Issues, "PROJECT_VERSION_CONFLICT", RuntimeNode) != 1 || countIssue(result.Issues, "PROJECT_VERSION_CONFLICT", RuntimeJava) != 1 {
		t.Fatalf("conflict issues = %#v", result.Issues)
	}
}

func TestIgnoresBuiltInExcludedAndSymlinkedTrees(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "workspace")
	writeFixture(t, filepath.Join(root, ".nvmrc"), "20\n")
	writeFixture(t, filepath.Join(root, "node_modules", "dependency", ".nvmrc"), "18\n")
	writeFixture(t, filepath.Join(root, "target", "generated", ".java-version"), "17\n")
	writeFixture(t, filepath.Join(root, "archived", ".nvmrc"), "16\n")
	external := filepath.Join(home, "external")
	writeFixture(t, filepath.Join(external, "package.json"), `{"engines":{"node":"14"}}`)
	if err := os.Symlink(external, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	result := Scan(context.Background(), Request{Roots: []string{root}, Excludes: []string{"archived"}, Home: home, CollectedAt: testTime})
	if len(result.Projects) != 1 || len(result.Projects[0].References) != 1 || result.Projects[0].References[0].Constraint != "20" {
		t.Fatalf("ignored tree result = %#v", result)
	}
}

func TestInvalidOversizedAndUnknownFilesAreSafe(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "workspace")
	writeFixture(t, filepath.Join(root, "package.json"), `{"name":"no-engine"}`)
	writeFixture(t, filepath.Join(root, "broken", "pom.xml"), `<project><properties>`)
	writeFixture(t, filepath.Join(root, "large", ".nvmrc"), strings.Repeat("x", (4<<10)+1))
	writeFixture(t, filepath.Join(root, "secret", ".node-version"), "TEST_TOKEN_SUPER_SECRET\n")
	writeFixture(t, filepath.Join(root, "dynamic", "build.gradle"), `sourceCompatibility = project.findProperty("javaVersion")`)

	result := Scan(context.Background(), Request{Roots: []string{root}, Home: home, CollectedAt: testTime})
	if countIssue(result.Issues, "PROJECT_FILE_INVALID", "") != 2 {
		t.Fatalf("invalid issues = %#v", result.Issues)
	}
	serialized := detailString(result)
	if strings.Contains(serialized, "TEST_TOKEN_SUPER_SECRET") {
		t.Fatalf("sensitive file content leaked: %s", serialized)
	}
	if !strings.Contains(serialized, "unknown") || !strings.Contains(serialized, "dynamic") {
		t.Fatalf("unknown declarations missing: %s", serialized)
	}
}

func TestDamagedParsersAndMissingFields(t *testing.T) {
	tests := []struct {
		name           string
		file           string
		body           string
		wantError      bool
		wantReferences int
	}{
		{name: "empty nvmrc", file: ".nvmrc", body: "# comment\n", wantError: true},
		{name: "package missing engines", file: "package.json", body: `{"name":"ok"}`},
		{name: "package damaged", file: "package.json", body: `{`, wantError: true},
		{name: "tool versions damaged", file: ".tool-versions", body: "nodejs\n", wantError: true},
		{name: "tool versions missing target", file: ".tool-versions", body: "python 3.12\n"},
		{name: "pom missing property", file: "pom.xml", body: `<project/>`},
		{name: "pom damaged", file: "pom.xml", body: `<project>`, wantError: true},
		{name: "gradle missing declaration", file: "build.gradle.kts", body: `plugins { java }`},
		{name: "gradle dynamic", file: "build.gradle", body: `sourceCompatibility = javaVersion`, wantReferences: 1},
		{name: "gradle legacy static", file: "build.gradle", body: `sourceCompatibility = JavaVersion.VERSION_1_8`, wantReferences: 1},
		{name: "gradle numeric static", file: "build.gradle", body: "sourceCompatibility = 17\n", wantReferences: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			references, err := parseFile(test.file, []byte(test.body))
			if (err != nil) != test.wantError || len(references) != test.wantReferences {
				t.Fatalf("references/error = %#v / %v", references, err)
			}
		})
	}
}

func TestMissingRootAndDepthLimitDegradeExplicitly(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "deep")
	deep := root
	for index := 0; index < maxDepth+1; index++ {
		deep = filepath.Join(deep, "level")
	}
	writeFixture(t, filepath.Join(deep, ".nvmrc"), "20\n")
	result := Scan(context.Background(), Request{Roots: []string{filepath.Join(home, "missing"), root}, Home: home, CollectedAt: testTime})
	if countIssue(result.Issues, "PROJECT_ROOT_INVALID", "") != 1 || countIssue(result.Issues, "PROJECT_SCAN_INCOMPLETE", "") != 1 {
		t.Fatalf("root/depth issues = %#v", result.Issues)
	}
	if len(result.Projects) != 0 {
		t.Fatalf("file below depth limit was scanned: %#v", result.Projects)
	}
}

func TestDirectoryAndFileCountLimitsAreEnforced(t *testing.T) {
	t.Run("directory count", func(t *testing.T) {
		home := t.TempDir()
		root := filepath.Join(home, "workspace")
		writeFixture(t, filepath.Join(root, "a", ".nvmrc"), "20\n")
		result := scanWithLimits(context.Background(), Request{Roots: []string{root}, Home: home, CollectedAt: testTime}, scanLimits{depth: maxDepth, directories: 1, files: maxFiles})
		if countIssue(result.Issues, "PROJECT_SCAN_INCOMPLETE", "") != 1 || len(result.Projects) != 0 {
			t.Fatalf("directory limit result = %#v", result)
		}
	})
	t.Run("file count", func(t *testing.T) {
		home := t.TempDir()
		root := filepath.Join(home, "workspace")
		writeFixture(t, filepath.Join(root, ".nvmrc"), "20\n")
		writeFixture(t, filepath.Join(root, ".node-version"), "20\n")
		result := scanWithLimits(context.Background(), Request{Roots: []string{root}, Home: home, CollectedAt: testTime}, scanLimits{depth: maxDepth, directories: maxDirectories, files: 1})
		if countIssue(result.Issues, "PROJECT_SCAN_INCOMPLETE", "") != 1 {
			t.Fatalf("file limit result = %#v", result)
		}
	})
}

func TestCancellationIsReportedAsIncomplete(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "workspace")
	writeFixture(t, filepath.Join(root, ".nvmrc"), "20\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Scan(ctx, Request{Roots: []string{root}, Home: home, CollectedAt: testTime})
	if countIssue(result.Issues, "PROJECT_SCAN_INCOMPLETE", "") != 1 || len(result.Projects) != 0 {
		t.Fatalf("cancelled result = %#v", result)
	}
}

func TestEquivalentJavaDeclarationsDoNotConflict(t *testing.T) {
	project := Project{Root: "$HOME/app", References: []Reference{
		javaReference("21", ".java-version"),
		javaReference("21.0.0", "pom.xml"),
		javaReference("1.8.0_361", "pom.xml"),
		javaReference("8u361", ".tool-versions"),
	}}
	issues := conflictIssues(project)
	if len(issues) != 1 || issues[0].Runtime != RuntimeJava {
		t.Fatalf("issues = %#v; only Java 21 versus Java 8 should conflict", issues)
	}
	project.References = project.References[:2]
	if issues := conflictIssues(project); len(issues) != 0 {
		t.Fatalf("equivalent Java declarations conflict: %#v", issues)
	}
}

func writeFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func countIssue(issues []Issue, code string, runtime Runtime) int {
	count := 0
	for _, issue := range issues {
		if issue.Code == code && (runtime == "" || issue.Runtime == runtime) {
			count++
		}
	}
	return count
}

func detailString(result Result) string {
	var output strings.Builder
	for _, project := range result.Projects {
		for _, reference := range project.References {
			output.WriteString(reference.Constraint)
			output.WriteString(reference.File)
		}
	}
	for _, issue := range result.Issues {
		output.WriteString(strings.Join(issue.Details, " "))
	}
	return output.String()
}
