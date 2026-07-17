package projectscan

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxDepth       = 32
	maxDirectories = 20_000
	maxFiles       = 2_000
)

type scanLimits struct{ depth, directories, files int }

var defaultScanLimits = scanLimits{depth: maxDepth, directories: maxDirectories, files: maxFiles}

var ignoredDirectories = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".gradle": true,
	"node_modules": true, "target": true, "build": true, "dist": true,
	"out": true, "coverage": true,
}

var fileLimits = map[string]int64{
	".nvmrc": 4 << 10, ".node-version": 4 << 10, ".java-version": 4 << 10,
	".tool-versions": 64 << 10, "package.json": 1 << 20, "pom.xml": 2 << 20,
	"build.gradle": 1 << 20, "build.gradle.kts": 1 << 20,
}

// Scan only traverses roots explicitly present in the request.
func Scan(ctx context.Context, request Request) Result {
	return scanWithLimits(ctx, request, defaultScanLimits)
}

func scanWithLimits(ctx context.Context, request Request, limits scanLimits) Result {
	result := Result{CollectedAt: request.CollectedAt.UTC(), Projects: []Project{}, Issues: []Issue{}}
	if len(request.Roots) == 0 {
		return result
	}
	if result.CollectedAt.IsZero() {
		result.CollectedAt = time.Now().UTC()
	}
	if request.WorkingDirectory == "" {
		request.WorkingDirectory, _ = os.Getwd()
	}
	if request.Home == "" {
		request.Home, _ = os.UserHomeDir()
	}
	if resolvedHome, err := filepath.EvalSymlinks(filepath.Clean(request.Home)); err == nil {
		request.Home = resolvedHome
	}
	projects := map[string]*Project{}
	seenFiles := map[string]bool{}
	for _, suppliedRoot := range request.Roots {
		if err := ctx.Err(); err != nil {
			result.Issues = append(result.Issues, Issue{Code: "PROJECT_SCAN_INCOMPLETE", Root: redactPath(suppliedRoot, request.Home), Details: []string{"scan cancelled"}})
			break
		}
		root, err := resolveRoot(suppliedRoot, request.WorkingDirectory)
		if err != nil {
			result.Issues = append(result.Issues, Issue{Code: "PROJECT_ROOT_INVALID", Root: redactPath(suppliedRoot, request.Home)})
			continue
		}
		scanRoot(ctx, root, request, limits, projects, seenFiles, &result)
	}
	for _, project := range projects {
		sort.Slice(project.References, func(i, j int) bool {
			if project.References[i].Runtime != project.References[j].Runtime {
				return project.References[i].Runtime < project.References[j].Runtime
			}
			if project.References[i].File != project.References[j].File {
				return project.References[i].File < project.References[j].File
			}
			return project.References[i].Constraint < project.References[j].Constraint
		})
		result.Projects = append(result.Projects, *project)
		result.Issues = append(result.Issues, conflictIssues(*project)...)
	}
	sort.Slice(result.Projects, func(i, j int) bool { return result.Projects[i].Root < result.Projects[j].Root })
	sort.Slice(result.Issues, func(i, j int) bool {
		if result.Issues[i].Root != result.Issues[j].Root {
			return result.Issues[i].Root < result.Issues[j].Root
		}
		if result.Issues[i].File != result.Issues[j].File {
			return result.Issues[i].File < result.Issues[j].File
		}
		return result.Issues[i].Code < result.Issues[j].Code
	})
	return result
}

func scanRoot(ctx context.Context, root string, request Request, limits scanLimits, projects map[string]*Project, seenFiles map[string]bool, result *Result) {
	directories, files := 0, 0
	cancellationReported := false
	excludes := resolvedExcludes(root, request.Excludes)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			if !cancellationReported {
				result.Issues = append(result.Issues, Issue{Code: "PROJECT_SCAN_INCOMPLETE", Root: redactPath(root, request.Home), Details: []string{"scan cancelled"}})
				cancellationReported = true
			}
			return filepath.SkipAll
		}
		relative, _ := filepath.Rel(root, path)
		publicRoot := redactPath(root, request.Home)
		if walkErr != nil {
			result.Issues = append(result.Issues, Issue{Code: "PROJECT_PATH_UNAVAILABLE", Root: publicRoot, File: cleanRelative(relative)})
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != root && (ignoredDirectories[entry.Name()] || excluded(path, excludes)) {
				return filepath.SkipDir
			}
			directories++
			if depth(relative) > limits.depth || directories > limits.directories {
				result.Issues = append(result.Issues, Issue{Code: "PROJECT_SCAN_INCOMPLETE", Root: publicRoot, Details: []string{"directory limit reached"}})
				return filepath.SkipAll
			}
			return nil
		}
		limit, recognized := fileLimits[entry.Name()]
		if !recognized || entry.Type()&os.ModeSymlink != 0 || excluded(path, excludes) || seenFiles[path] {
			return nil
		}
		seenFiles[path] = true
		files++
		if files > limits.files {
			result.Issues = append(result.Issues, Issue{Code: "PROJECT_SCAN_INCOMPLETE", Root: publicRoot, Details: []string{"file limit reached"}})
			return filepath.SkipAll
		}
		body, err := readBounded(path, limit)
		if err != nil {
			code := "PROJECT_FILE_UNAVAILABLE"
			if errors.Is(err, errTooLarge) {
				code = "PROJECT_FILE_INVALID"
			}
			result.Issues = append(result.Issues, Issue{Code: code, Root: publicRoot, File: cleanRelative(relative)})
			return nil
		}
		references, err := parseFile(entry.Name(), body)
		if err != nil {
			result.Issues = append(result.Issues, Issue{Code: "PROJECT_FILE_INVALID", Root: publicRoot, File: cleanRelative(relative)})
			return nil
		}
		if len(references) == 0 {
			return nil
		}
		projectDirectory := filepath.Dir(path)
		project := projects[projectDirectory]
		if project == nil {
			project = &Project{ID: projectID(projectDirectory), Root: redactPath(projectDirectory, request.Home), References: []Reference{}}
			projects[projectDirectory] = project
		}
		fileInProject, _ := filepath.Rel(projectDirectory, path)
		for index := range references {
			references[index].File = cleanRelative(fileInProject)
			project.References = append(project.References, references[index])
		}
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		result.Issues = append(result.Issues, Issue{Code: "PROJECT_SCAN_INCOMPLETE", Root: redactPath(root, request.Home)})
	}
}

var errTooLarge = errors.New("project file exceeds size limit")

func readBounded(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("project file is not regular")
	}
	if info.Size() > limit {
		return nil, errTooLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errTooLarge
	}
	return body, nil
}

func resolveRoot(root, workingDirectory string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("empty project root")
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(workingDirectory, root)
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", errors.New("project root is not a directory")
	}
	return root, nil
}

func resolvedExcludes(root string, values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(root, value)
		}
		result = append(result, filepath.Clean(value))
	}
	return result
}

func excluded(path string, excludes []string) bool {
	path = filepath.Clean(path)
	for _, exclude := range excludes {
		if path == exclude || strings.HasPrefix(path, exclude+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func depth(relative string) int {
	if relative == "." {
		return 0
	}
	return strings.Count(filepath.Clean(relative), string(filepath.Separator)) + 1
}

func cleanRelative(value string) string { return filepath.ToSlash(filepath.Clean(value)) }

func projectID(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return fmt.Sprintf("project:%x", sum[:6])
}

func redactPath(path, home string) string {
	clean := filepath.Clean(path)
	if home == "" {
		return clean
	}
	home = filepath.Clean(home)
	relative, err := filepath.Rel(home, clean)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		if relative == "." {
			return "$HOME"
		}
		return filepath.ToSlash(filepath.Join("$HOME", relative))
	}
	return filepath.ToSlash(clean)
}
