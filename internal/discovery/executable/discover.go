package executable

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

var errSymlinkLoop = errors.New("symbolic link loop")

type dependencies struct {
	files         fileSystem
	architectures architectureInspector
}

// Discover finds command candidates without executing them or changing the
// filesystem. The caller supplies PATH directories in their original order.
func Discover(ctx context.Context, request Request) (Result, error) {
	return discover(ctx, request, dependencies{
		files:         osFileSystem{},
		architectures: machoInspector{},
	})
}

func discover(ctx context.Context, request Request, deps dependencies) (Result, error) {
	if err := validateCommand(request.Command); err != nil {
		return Result{}, err
	}
	if request.WorkingDirectory == "" {
		return Result{}, errors.New("working directory is required")
	}
	if !filepath.IsAbs(request.WorkingDirectory) {
		return Result{}, errors.New("working directory must be absolute")
	}
	workingDirectory := filepath.Clean(request.WorkingDirectory)
	collectedAt := request.CollectedAt.UTC()
	if request.CollectedAt.IsZero() {
		return Result{}, errors.New("collection time is required")
	}

	result := Result{
		Command:    request.Command,
		Candidates: []Candidate{},
		Findings:   []inventory.Finding{},
	}
	collector := &findingCollector{result: &result, collectedAt: collectedAt, home: request.Home}

	for position, directory := range request.Directories {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		resolvedDirectory := resolveDirectory(directory, workingDirectory)
		if directory == "" {
			collector.add(
				"EMPTY_PATH_ENTRY",
				inventory.SeverityWarning,
				"An empty PATH entry refers to the working directory.",
				[]string{redactHome(workingDirectory, request.Home)},
				inventory.ConfidenceHigh,
			)
		}
		candidatePath := filepath.Join(resolvedDirectory, request.Command)
		candidate, found := inspectCandidate(candidatePath, position, deps, collector)
		if found {
			result.Candidates = append(result.Candidates, candidate)
		}
	}

	markDuplicatesAndEffective(&result)
	if countUsable(result.Candidates) > 1 {
		evidence := make([]string, 0, len(result.Candidates))
		for _, candidate := range result.Candidates {
			if candidate.Executable && candidate.LinkState != LinkStateBroken && candidate.LinkState != LinkStateLoop {
				evidence = append(evidence, candidate.Path)
			}
		}
		collector.add(
			"EXECUTABLE_PATH_SHADOWED",
			inventory.SeverityWarning,
			"Multiple executable instances were found in PATH order.",
			evidence,
			inventory.ConfidenceHigh,
		)
	}
	return result, nil
}

func validateCommand(command string) error {
	if command == "" {
		return errors.New("command is required")
	}
	if command == "." || command == ".." || strings.ContainsAny(command, "/\\\x00") {
		return errors.New("command must be a single path segment")
	}
	for _, character := range command {
		if unicode.IsControl(character) {
			return errors.New("command must not contain control characters")
		}
	}
	return nil
}

func resolveDirectory(directory, workingDirectory string) string {
	if directory == "" {
		return workingDirectory
	}
	if filepath.IsAbs(directory) {
		return filepath.Clean(directory)
	}
	return filepath.Clean(filepath.Join(workingDirectory, directory))
}

func inspectCandidate(path string, position int, deps dependencies, findings *findingCollector) (Candidate, bool) {
	info, err := deps.files.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Candidate{}, false
		}
		code := "EXECUTABLE_ACCESS_FAILED"
		message := "A PATH candidate could not be inspected."
		if errors.Is(err, fs.ErrPermission) {
			code = "EXECUTABLE_PERMISSION_DENIED"
			message = "Permission was denied while inspecting a PATH candidate."
		}
		findings.add(code, inventory.SeverityWarning, message, []string{findings.redact(path)}, inventory.ConfidenceHigh)
		return Candidate{}, false
	}

	candidate := Candidate{
		DirectoryPosition: position,
		Path:              findings.redact(path),
		ResolvedPath:      findings.redact(path),
		LinkState:         LinkStateNotLink,
		Architectures:     []inventory.Architecture{inventory.ArchitectureUnknown},
		accessPath:        path,
		invocationPath:    path,
	}
	resolvedPath := path
	resolvedInfo := info
	resolvedPath, err = deps.files.EvalSymlinks(path)
	if err != nil {
		candidate.ResolvedPath = UnknownPath
		switch {
		case isSymlinkLoop(err):
			candidate.LinkState = LinkStateLoop
			findings.add("EXECUTABLE_LINK_LOOP", inventory.SeverityWarning, "A PATH candidate contains a symbolic-link loop.", []string{candidate.Path}, inventory.ConfidenceHigh)
		case errors.Is(err, fs.ErrNotExist):
			candidate.LinkState = LinkStateBroken
			findings.add("EXECUTABLE_LINK_BROKEN", inventory.SeverityWarning, "A PATH candidate is a broken symbolic link.", []string{candidate.Path}, inventory.ConfidenceHigh)
		default:
			candidate.LinkState = LinkStateUnknown
			findings.add("EXECUTABLE_LINK_UNRESOLVED", inventory.SeverityWarning, "A PATH candidate symbolic link could not be resolved.", []string{candidate.Path}, inventory.ConfidenceLow)
		}
		return candidate, true
	}
	if info.Mode()&fs.ModeSymlink != 0 || filepath.Clean(resolvedPath) != filepath.Clean(path) {
		candidate.LinkState = LinkStateResolved
		candidate.ResolvedPath = findings.redact(resolvedPath)
		candidate.accessPath = resolvedPath
	}
	if filepath.Clean(resolvedPath) != filepath.Clean(path) || info.Mode()&fs.ModeSymlink != 0 {
		resolvedInfo, err = deps.files.Stat(resolvedPath)
		if err != nil {
			candidate.LinkState = LinkStateUnknown
			candidate.ResolvedPath = UnknownPath
			findings.add("EXECUTABLE_TARGET_ACCESS_FAILED", inventory.SeverityWarning, "A resolved PATH candidate could not be inspected.", []string{candidate.Path}, inventory.ConfidenceLow)
			return candidate, true
		}
	}

	candidate.Executable = resolvedInfo.Mode().IsRegular() && resolvedInfo.Mode().Perm()&0o111 != 0
	if !candidate.Executable {
		findings.add("EXECUTABLE_NOT_EXECUTABLE", inventory.SeverityWarning, "A PATH candidate is not an executable regular file.", []string{candidate.Path}, inventory.ConfidenceHigh)
		return candidate, true
	}
	architectures, err := deps.architectures.Inspect(resolvedPath)
	if err == nil {
		candidate.Architectures = normalizeArchitectures(architectures)
	} else if errors.Is(err, fs.ErrPermission) {
		findings.add("EXECUTABLE_ARCHITECTURE_PERMISSION_DENIED", inventory.SeverityWarning, "Permission was denied while reading executable architecture metadata.", []string{candidate.ResolvedPath}, inventory.ConfidenceHigh)
	}
	return candidate, true
}

func markDuplicatesAndEffective(result *Result) {
	counts := make(map[string]int, len(result.Candidates))
	for _, candidate := range result.Candidates {
		counts[candidate.Path]++
	}
	effectiveMarked := false
	for index := range result.Candidates {
		candidate := &result.Candidates[index]
		candidate.Duplicate = counts[candidate.Path] > 1
		usable := candidate.Executable && candidate.LinkState != LinkStateBroken && candidate.LinkState != LinkStateLoop && candidate.LinkState != LinkStateUnknown
		if usable && !effectiveMarked {
			candidate.Effective = true
			effectiveMarked = true
		}
	}
}

func countUsable(candidates []Candidate) int {
	count := 0
	for _, candidate := range candidates {
		if candidate.Executable && candidate.LinkState != LinkStateBroken && candidate.LinkState != LinkStateLoop && candidate.LinkState != LinkStateUnknown {
			count++
		}
	}
	return count
}

type findingCollector struct {
	result      *Result
	collectedAt time.Time
	home        string
}

func (c *findingCollector) add(code string, severity inventory.FindingSeverity, message string, evidence []string, confidence inventory.Confidence) {
	index := len(c.result.Findings) + 1
	source := inventory.SourceMetadata{
		Kind:        inventory.SourceFile,
		Name:        "PATH executable discovery",
		CollectedAt: c.collectedAt,
		Confidence:  confidence,
	}
	c.result.Findings = append(c.result.Findings, inventory.Finding{
		ID:         fmt.Sprintf("executable-discovery-%d", index),
		Code:       code,
		Severity:   severity,
		Message:    message,
		Evidence:   evidence,
		Confidence: confidence,
		Sources:    []inventory.SourceMetadata{source},
	})
}

func (c *findingCollector) redact(path string) string {
	return redactHome(path, c.home)
}

func redactHome(path, home string) string {
	if home == "" {
		return path
	}
	cleanHome := filepath.Clean(home)
	cleanPath := filepath.Clean(path)
	if cleanPath == cleanHome {
		return "$HOME"
	}
	prefix := cleanHome + string(filepath.Separator)
	if strings.HasPrefix(cleanPath, prefix) {
		return "$HOME" + strings.TrimPrefix(cleanPath, cleanHome)
	}
	return cleanPath
}
