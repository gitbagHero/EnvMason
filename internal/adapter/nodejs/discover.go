package nodejs

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/discovery/executable"
	"github.com/gitbagHero/EnvMason/internal/inventory"
)

const unknown = "unknown"

var packageManagerNames = []string{"npm", "corepack", "pnpm", "yarn"}

type dependencies struct {
	runner commandRunner
}

type nodeRecord struct {
	installation NodeInstallation
	invocation   string
	resolved     string
}

// Discover performs bounded, read-only Node.js ecosystem discovery. It does
// not source shell profiles or invoke nvm.
func Discover(ctx context.Context, request Request) (Result, error) {
	return discover(ctx, request, dependencies{runner: execRunner{}})
}

func discover(ctx context.Context, request Request, deps dependencies) (Result, error) {
	if request.CollectedAt.IsZero() {
		return Result{}, errors.New("collection time is required")
	}
	result := Result{
		State: StateNotInstalled,
		NVM:   NVM{State: StateNotInstalled, Directory: unknown, DefaultAlias: unknown, DefaultVersion: unknown},
		Nodes: []NodeInstallation{}, PackageManagers: []PackageManager{}, Findings: []inventory.Finding{},
	}
	collector := findingCollector{result: &result, collectedAt: request.CollectedAt.UTC()}

	pathNodes, err := discoverCommand(ctx, "node", request.PathDirectories, request)
	if err != nil {
		return Result{}, fmt.Errorf("discover node in PATH: %w", err)
	}
	collector.append(pathNodes.Findings)

	nvmDirectory, nvmState := locateNVMDirectory(request, &collector)
	result.NVM.State = nvmState
	if nvmDirectory != "" {
		result.NVM.Directory = redactHome(nvmDirectory, request.Home)
	}

	records := make([]nodeRecord, 0, len(pathNodes.Candidates)+4)
	seenNodes := make(map[string]bool)
	appendCandidate := func(candidate executable.Candidate, inPATH bool) {
		if !candidate.Executable || candidate.AccessPath() == "" {
			return
		}
		key := filepath.Clean(candidate.AccessPath())
		if seenNodes[key] {
			if inPATH {
				for index := range records {
					if filepath.Clean(records[index].resolved) == key {
						records[index].installation.InPATH = true
						records[index].installation.Effective = candidate.Effective
					}
				}
			}
			return
		}
		seenNodes[key] = true
		manager := managerForPath(candidate.InvocationPath(), nvmDirectory, request.HomebrewPrefixes)
		version := nvmVersionForPath(candidate.InvocationPath(), nvmDirectory)
		if version == "" {
			output, runErr := deps.runner.Run(ctx, candidate.InvocationPath(), []string{"--version"}, request.WorkingDirectory, safeVersionEnvironment(request))
			if runErr != nil {
				version = unknown
				collector.add("NODE_VERSION_QUERY_FAILED", "A Node.js version query failed.", []string{candidate.Path})
			} else {
				version = normalizeNodeVersion(output)
				if version == unknown {
					collector.add("NODE_VERSION_OUTPUT_INVALID", "A Node.js version query returned an unrecognized value.", []string{candidate.Path})
				}
			}
		}
		architecture := firstKnownArchitecture(candidate.Architectures, request.ProcessArchitecture)
		normalized := strings.TrimPrefix(version, "v")
		if version == unknown {
			normalized = ""
		}
		installation := NodeInstallation{
			ID: nodeID(manager, candidate.ResolvedPath), Version: version, NormalizedVersion: normalized,
			Path: candidate.Path, ResolvedPath: candidate.ResolvedPath, Architecture: architecture,
			Manager: manager, Effective: inPATH && candidate.Effective, InPATH: inPATH,
		}
		records = append(records, nodeRecord{installation: installation, invocation: candidate.InvocationPath(), resolved: candidate.AccessPath()})
	}
	for _, candidate := range pathNodes.Candidates {
		appendCandidate(candidate, true)
	}

	var nvmVersions []string
	if nvmDirectory != "" && nvmState == StateInstalled {
		nvmVersions, err = installedNVMVersions(nvmDirectory)
		if err != nil {
			collector.add("NVM_VERSIONS_UNAVAILABLE", "NVM versions could not be read.", []string{result.NVM.Directory})
		} else {
			for _, version := range nvmVersions {
				binDirectory := filepath.Join(nvmDirectory, "versions", "node", version, "bin")
				discovered, discoverErr := discoverCommand(ctx, "node", []string{binDirectory}, request)
				if discoverErr != nil {
					collector.add("NVM_NODE_DISCOVERY_FAILED", "An NVM Node.js installation could not be inspected.", []string{redactHome(binDirectory, request.Home)})
					continue
				}
				collector.append(discovered.Findings)
				for _, candidate := range discovered.Candidates {
					appendCandidate(candidate, false)
				}
			}
		}
		alias, defaultVersion, aliasErr := resolveDefaultAlias(nvmDirectory, nvmVersions)
		if aliasErr != nil {
			collector.add("NVM_DEFAULT_ALIAS_UNAVAILABLE", "The NVM default alias could not be resolved to an installed version.", []string{result.NVM.Directory})
		} else {
			result.NVM.DefaultAlias = alias
			result.NVM.DefaultVersion = defaultVersion
			for index := range records {
				if records[index].installation.Manager == ManagerNVM && records[index].installation.Version == defaultVersion {
					records[index].installation.Default = true
				}
			}
		}
	}

	for _, record := range records {
		if record.installation.Effective {
			result.CurrentNodeID = record.installation.ID
		}
		result.Nodes = append(result.Nodes, record.installation)
	}
	if len(result.Nodes) > 0 {
		result.State = StateInstalled
	}
	sort.Slice(result.Nodes, func(i, j int) bool {
		if result.Nodes[i].Effective != result.Nodes[j].Effective {
			return result.Nodes[i].Effective
		}
		if result.Nodes[i].Manager != result.Nodes[j].Manager {
			return result.Nodes[i].Manager < result.Nodes[j].Manager
		}
		return result.Nodes[i].Version < result.Nodes[j].Version
	})

	result.NVM.Loaded = nvmDirectory != "" && request.NVMBin != "" && pathWithin(request.NVMBin, nvmDirectory)
	if result.NVM.State == StateInstalled && !result.NVM.Loaded {
		collector.add("NVM_NOT_LOADED", "NVM was found on disk but is not loaded in the current shell environment.", []string{result.NVM.Directory})
	}
	if multipleManagers(result.Nodes) {
		collector.add("NODE_MULTIPLE_SOURCES", "Node.js installations from multiple managers were found.", nodeEvidence(result.Nodes))
	}

	packages := discoverPackageManagers(ctx, request, deps, records, nvmDirectory, &collector)
	result.PackageManagers = packages
	return result, nil
}

func locateNVMDirectory(request Request, findings *findingCollector) (string, State) {
	candidates := []string{}
	if request.NVMDirectory != "" {
		candidates = append(candidates, request.NVMDirectory)
	} else if request.XDGConfigHome != "" {
		candidates = append(candidates, filepath.Join(request.XDGConfigHome, "nvm"))
	} else if request.Home != "" {
		candidates = append(candidates, filepath.Join(request.Home, ".nvm"))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.IsDir():
			return filepath.Clean(candidate), StateInstalled
		case err == nil:
			findings.add("NVM_PATH_NOT_DIRECTORY", "The configured NVM path is not a directory.", []string{redactHome(candidate, request.Home)})
			return "", StateUnknown
		case errors.Is(err, fs.ErrNotExist):
			continue
		default:
			findings.add("NVM_PATH_UNAVAILABLE", "The configured NVM path could not be inspected.", []string{redactHome(candidate, request.Home)})
			return "", StateUnknown
		}
	}
	return "", StateNotInstalled
}

func discoverCommand(ctx context.Context, command string, directories []string, request Request) (executable.Result, error) {
	return executable.Discover(ctx, executable.Request{
		Command: command, Directories: directories, WorkingDirectory: request.WorkingDirectory,
		Home: request.Home, CollectedAt: request.CollectedAt,
	})
}

func discoverPackageManagers(ctx context.Context, request Request, deps dependencies, nodes []nodeRecord, nvmDirectory string, findings *findingCollector) []PackageManager {
	result := []PackageManager{}
	seen := make(map[string]bool)
	appendCandidate := func(name string, candidate executable.Candidate, effective bool) {
		if !candidate.Executable || candidate.AccessPath() == "" {
			return
		}
		key := name + "\x00" + filepath.Clean(candidate.AccessPath())
		if seen[key] {
			if effective {
				for index := range result {
					if result[index].Name == name && result[index].ResolvedPath == candidate.ResolvedPath {
						result[index].Effective = true
					}
				}
			}
			return
		}
		seen[key] = true
		ownerID := ownerForPackageCandidate(candidate, nodes)
		manager := managerForPath(candidate.InvocationPath(), nvmDirectory, request.HomebrewPrefixes)
		version := unknown
		providerVersion := ""
		corepackProxy := (name == "pnpm" || name == "yarn") && isCorepackExecutable(candidate.AccessPath())
		if metadata, ok := metadataForExecutable(candidate.AccessPath()); ok && metadataMatchesCommand(name, metadata) && validVersionOutput(metadata.Version) {
			if corepackProxy {
				providerVersion = metadata.Version
			} else {
				version = metadata.Version
			}
		}
		if version == unknown && !corepackProxy {
			output, err := deps.runner.Run(ctx, candidate.InvocationPath(), []string{"--version"}, request.WorkingDirectory, safeVersionEnvironment(request))
			if err != nil {
				findings.add("PACKAGE_MANAGER_VERSION_QUERY_FAILED", "A package-manager version query failed.", []string{candidate.Path})
			} else if value := firstOutputLine(output); validVersionOutput(value) {
				version = value
			} else {
				findings.add("PACKAGE_MANAGER_VERSION_OUTPUT_INVALID", "A package-manager version query returned an unrecognized value.", []string{candidate.Path})
			}
		}
		result = append(result, PackageManager{
			Name: name, Version: version, Path: candidate.Path, ResolvedPath: candidate.ResolvedPath,
			Manager: manager, NodeInstallationID: ownerID, Effective: effective, CorepackProxy: corepackProxy,
			ProviderVersion: providerVersion,
		})
	}

	for _, name := range packageManagerNames {
		discovered, err := discoverCommand(ctx, name, request.PathDirectories, request)
		if err != nil {
			findings.add("PACKAGE_MANAGER_DISCOVERY_FAILED", "A package manager could not be discovered in PATH.", []string{name})
			continue
		}
		findings.append(discovered.Findings)
		for _, candidate := range discovered.Candidates {
			appendCandidate(name, candidate, candidate.Effective)
		}
		for _, node := range nodes {
			binDirectory := filepath.Dir(node.invocation)
			offline, offlineErr := discoverCommand(ctx, name, []string{binDirectory}, request)
			if offlineErr != nil {
				continue
			}
			findings.append(offline.Findings)
			for _, candidate := range offline.Candidates {
				appendCandidate(name, candidate, false)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		if result[i].Effective != result[j].Effective {
			return result[i].Effective
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func ownerForPackageCandidate(candidate executable.Candidate, nodes []nodeRecord) string {
	for _, node := range nodes {
		if filepath.Clean(filepath.Dir(candidate.InvocationPath())) == filepath.Clean(filepath.Dir(node.invocation)) {
			return node.installation.ID
		}
	}
	for _, node := range nodes {
		if node.installation.Effective {
			return node.installation.ID
		}
	}
	return ""
}

func managerForPath(path, nvmDirectory string, homebrewPrefixes []string) Manager {
	if nvmDirectory != "" && pathWithin(path, nvmDirectory) {
		return ManagerNVM
	}
	for _, prefix := range homebrewPrefixes {
		if prefix != "" && pathWithin(path, prefix) {
			return ManagerHomebrew
		}
	}
	if path != "" {
		return ManagerSystem
	}
	return ManagerUnknown
}

func nvmVersionForPath(path, nvmDirectory string) string {
	if nvmDirectory == "" || !pathWithin(path, nvmDirectory) {
		return ""
	}
	relative, err := filepath.Rel(filepath.Join(nvmDirectory, "versions", "node"), path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	if len(parts) >= 3 && parts[1] == "bin" && parts[2] == "node" && validNodeVersion(parts[0]) {
		return parts[0]
	}
	return ""
}

func normalizeNodeVersion(output string) string {
	value := firstOutputLine(output)
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	if !validNodeVersion(value) {
		return unknown
	}
	return value
}

func firstOutputLine(output string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(line)
}

func validVersionOutput(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for index, character := range value {
		switch {
		case character >= '0' && character <= '9':
		case character >= 'A' && character <= 'Z':
		case character >= 'a' && character <= 'z':
		case index > 0 && strings.ContainsRune(".+_-", character):
		default:
			return false
		}
	}
	return true
}

func firstKnownArchitecture(values []inventory.Architecture, fallback inventory.Architecture) inventory.Architecture {
	for _, value := range values {
		if value != inventory.ArchitectureUnknown {
			return value
		}
	}
	if fallback != "" {
		return fallback
	}
	return inventory.ArchitectureUnknown
}

func safeVersionEnvironment(request Request) map[string]string {
	return map[string]string{
		"PATH": strings.Join(request.PathDirectories, string(os.PathListSeparator)),
		"LANG": "C", "LC_ALL": "C", "NO_COLOR": "1",
		"COREPACK_ENABLE_NETWORK": "0", "COREPACK_DEFAULT_TO_LATEST": "0",
		"COREPACK_ENABLE_AUTO_PIN": "0", "COREPACK_ENABLE_PROJECT_SPEC": "0",
		"COREPACK_ENABLE_DOWNLOAD_PROMPT": "0", "NO_UPDATE_NOTIFIER": "1",
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
	}
}

func pathWithin(path, parent string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func redactHome(path, home string) string {
	if path == "" || home == "" {
		return path
	}
	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if cleanPath == cleanHome {
		return "$HOME"
	}
	if pathWithin(cleanPath, cleanHome) {
		return "$HOME" + strings.TrimPrefix(cleanPath, cleanHome)
	}
	return cleanPath
}

func nodeID(manager Manager, resolvedPath string) string {
	sum := sha256.Sum256([]byte(string(manager) + "\x00" + resolvedPath))
	return fmt.Sprintf("node:%s:%x", manager, sum[:6])
}

func multipleManagers(nodes []NodeInstallation) bool {
	values := make(map[Manager]bool)
	for _, node := range nodes {
		values[node.Manager] = true
	}
	return len(values) > 1
}

func nodeEvidence(nodes []NodeInstallation) []string {
	result := make([]string, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, node.Path)
	}
	return result
}

type findingCollector struct {
	result      *Result
	collectedAt time.Time
}

func (c *findingCollector) append(findings []inventory.Finding) {
	for _, finding := range findings {
		finding.ID = fmt.Sprintf("nodejs-adapter-%d", len(c.result.Findings)+1)
		c.result.Findings = append(c.result.Findings, finding)
	}
}

func (c *findingCollector) add(code, message string, evidence []string) {
	source := inventory.SourceMetadata{Kind: inventory.SourceFile, Name: "Node.js ecosystem discovery", CollectedAt: c.collectedAt, Confidence: inventory.ConfidenceHigh}
	c.result.Findings = append(c.result.Findings, inventory.Finding{
		ID: fmt.Sprintf("nodejs-adapter-%d", len(c.result.Findings)+1), Code: code,
		Severity: inventory.SeverityWarning, Message: message, Evidence: evidence,
		Confidence: inventory.ConfidenceHigh, Sources: []inventory.SourceMetadata{source},
	})
}
