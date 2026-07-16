package java

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/discovery/executable"
	"github.com/gitbagHero/EnvMason/internal/inventory"
)

const (
	unknown        = "unknown"
	maxVersionFile = 4 * 1024
)

type dependencies struct {
	runner commandRunner
}

type jdkRecord struct {
	installation JDKInstallation
	home         string
}

// Discover performs bounded, read-only Java ecosystem discovery on macOS.
func Discover(ctx context.Context, request Request) (Result, error) {
	return discover(ctx, request, dependencies{runner: execRunner{}})
}

func discover(ctx context.Context, request Request, deps dependencies) (Result, error) {
	if request.CollectedAt.IsZero() {
		return Result{}, errors.New("collection time is required")
	}
	result := Result{
		State: StateNotInstalled, JavaHome: redactUnknown(request.JavaHome, request.Home), JDKs: []JDKInstallation{},
		Current: unknownRuntime(),
		Jenv:    Jenv{State: StateNotInstalled, Root: unknown, GlobalVersion: unknown, LocalVersion: unknown, LocalVersionFile: unknown, ShellVersion: redactUnknown(request.JenvShellVersion, request.Home), EffectiveVersion: unknown, Registrations: []JenvRegistration{}},
		Maven:   unknownBuildTool("maven"), Gradle: unknownBuildTool("gradle"), Findings: []inventory.Finding{},
	}
	collector := findingCollector{result: &result, collectedAt: request.CollectedAt.UTC()}
	records := []jdkRecord{}
	seenJDKs := make(map[string]int)
	appendJDK := func(installation JDKInstallation, accessHome string) {
		if installation.Architecture == inventory.ArchitectureUnknown && request.ProcessArchitecture != "" {
			installation.Architecture = request.ProcessArchitecture
		}
		key := canonicalPath(accessHome)
		if index, ok := seenJDKs[key]; ok {
			existing := &records[index].installation
			existing.Registered = existing.Registered || installation.Registered
			existing.JenvAliases = appendUnique(existing.JenvAliases, installation.JenvAliases...)
			if existing.Manager == ManagerUnknown && installation.Manager != ManagerUnknown {
				existing.Manager = installation.Manager
			}
			return
		}
		installation.Home = redactHome(accessHome, request.Home)
		installation.ID = jdkID(installation.Home)
		installation.JenvAliases = append([]string{}, installation.JenvAliases...)
		seenJDKs[key] = len(records)
		records = append(records, jdkRecord{installation: installation, home: accessHome})
	}

	javaHomeTool := request.JavaHomeTool
	if javaHomeTool == "" {
		javaHomeTool = "/usr/libexec/java_home"
	}
	execDirectory := executionDirectory(request)
	if output, err := deps.runner.Run(ctx, javaHomeTool, []string{"-X"}, execDirectory, safeEnvironment(request)); err != nil {
		collector.add("JAVA_HOME_REGISTRY_QUERY_FAILED", "The macOS Java registry could not be queried.", []string{javaHomeTool})
	} else if installations, parseErr := parseJavaHomeXML(output, request.Home); parseErr != nil {
		collector.add("JAVA_HOME_REGISTRY_OUTPUT_INVALID", "The macOS Java registry returned invalid structured output.", []string{javaHomeTool})
	} else {
		for _, installation := range installations {
			accessHome := restoreHome(installation.Home, request.Home)
			appendJDK(installation, accessHome)
		}
	}

	discoverHomebrewJDKs(request, &collector, appendJDK)
	discoverJenv(request, &result, &collector, appendJDK)
	if request.JavaHome != "" {
		if version, vendor, architecture, err := parseReleaseFile(request.JavaHome); err == nil {
			appendJDK(JDKInstallation{Version: version, Name: "JAVA_HOME", Vendor: vendor, Architecture: architecture, Manager: managerForHome(request.JavaHome, request.HomebrewPrefixes), JenvAliases: []string{}}, request.JavaHome)
		} else {
			collector.add("JAVA_HOME_INVALID", "JAVA_HOME does not point to readable JDK release metadata.", []string{redactHome(request.JavaHome, request.Home)})
		}
	}

	javaResult, err := discoverCommand(ctx, "java", request)
	if err != nil {
		return Result{}, fmt.Errorf("discover java: %w", err)
	}
	collector.append(javaResult.Findings)
	if candidate := effectiveCandidate(javaResult); candidate != nil {
		output, runErr := deps.runner.Run(ctx, candidate.InvocationPath(), []string{"-XshowSettings:properties", "-version"}, execDirectory, safeEnvironment(request))
		if runErr != nil {
			result.Current.State = StateUnknown
			collector.add("JAVA_RUNTIME_QUERY_FAILED", "The effective Java runtime could not be queried.", []string{candidate.Path})
		} else {
			result.Current = parseJavaRuntime(output, request.Home)
		}
		result.Current.Path = candidate.Path
		result.Current.ResolvedPath = candidate.ResolvedPath
		result.Current.JDKID = matchJDKID(result.Current.Home, records, request.Home)
	}

	result.Maven = discoverBuildTool(ctx, "mvn", []string{"--version"}, request, deps, &collector, parseMaven)
	result.Gradle = discoverGradle(ctx, request, result.Current, &collector)

	for _, record := range records {
		sort.Strings(record.installation.JenvAliases)
		result.JDKs = append(result.JDKs, record.installation)
	}
	sort.Slice(result.JDKs, func(i, j int) bool {
		if result.JDKs[i].Version != result.JDKs[j].Version {
			return result.JDKs[i].Version < result.JDKs[j].Version
		}
		return result.JDKs[i].Home < result.JDKs[j].Home
	})
	if len(result.JDKs) > 0 || result.Current.State == StateInstalled {
		result.State = StateInstalled
	}
	addConflictFindings(request, &result, &collector)
	return result, nil
}

func discoverHomebrewJDKs(request Request, findings *findingCollector, appendJDK func(JDKInstallation, string)) {
	for _, prefix := range request.HomebrewPrefixes {
		optDirectory := filepath.Join(prefix, "opt")
		entries, err := os.ReadDir(optDirectory)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				findings.add("HOMEBREW_JDK_DIRECTORY_UNAVAILABLE", "A Homebrew opt directory could not be read.", []string{redactHome(optDirectory, request.Home)})
			}
			continue
		}
		for _, entry := range entries {
			name := strings.ToLower(entry.Name())
			if name != "java" && !strings.HasPrefix(name, "openjdk") {
				continue
			}
			optPath := filepath.Join(optDirectory, entry.Name())
			resolved, resolveErr := filepath.EvalSymlinks(optPath)
			if resolveErr != nil {
				findings.add("HOMEBREW_JDK_LINK_BROKEN", "A Homebrew JDK opt link could not be resolved.", []string{redactHome(optPath, request.Home)})
				continue
			}
			candidates := []string{filepath.Join(resolved, "libexec", "openjdk.jdk", "Contents", "Home"), resolved}
			for _, candidate := range candidates {
				version, vendor, architecture, metadataErr := parseReleaseFile(candidate)
				if metadataErr != nil {
					continue
				}
				appendJDK(JDKInstallation{Version: version, Name: entry.Name(), Vendor: vendor, Architecture: architecture, Manager: ManagerHomebrew, JenvAliases: []string{}}, candidate)
				break
			}
		}
	}
}

func discoverJenv(request Request, result *Result, findings *findingCollector, appendJDK func(JDKInstallation, string)) {
	root := request.JenvRoot
	if root == "" && request.Home != "" {
		root = filepath.Join(request.Home, ".jenv")
	}
	if root == "" {
		return
	}
	info, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil || !info.IsDir() {
		result.Jenv.State = StateUnknown
		findings.add("JENV_ROOT_UNAVAILABLE", "The jenv root could not be inspected.", []string{redactHome(root, request.Home)})
		return
	}
	result.Jenv.State = StateInstalled
	result.Jenv.Root = redactHome(root, request.Home)
	result.Jenv.Loaded = containsDirectory(request.PathDirectories, filepath.Join(root, "shims"))
	versionsDirectory := filepath.Join(root, "versions")
	entries, readErr := os.ReadDir(versionsDirectory)
	if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
		findings.add("JENV_VERSIONS_UNAVAILABLE", "jenv registrations could not be read.", []string{redactHome(versionsDirectory, request.Home)})
	}
	for _, entry := range entries {
		alias := entry.Name()
		if !validSelection(alias) {
			findings.add("JENV_ALIAS_INVALID", "A jenv registration has an invalid alias name.", []string{redactHome(versionsDirectory, request.Home)})
			continue
		}
		path := filepath.Join(versionsDirectory, alias)
		resolved, resolveErr := filepath.EvalSymlinks(path)
		registration := JenvRegistration{Alias: alias, Home: unknown}
		if resolveErr != nil {
			registration.Broken = true
			findings.add("JENV_REGISTRATION_BROKEN", "A jenv registration points to a missing or looping target.", []string{redactHome(path, request.Home)})
		} else {
			registration.Home = redactHome(resolved, request.Home)
			version, vendor, architecture, metadataErr := parseReleaseFile(resolved)
			if metadataErr != nil {
				findings.add("JENV_REGISTRATION_INVALID", "A jenv registration does not point to readable JDK metadata.", []string{registration.Home})
			} else {
				appendJDK(JDKInstallation{Version: version, Name: alias, Vendor: vendor, Architecture: architecture, Manager: managerForHome(resolved, request.HomebrewPrefixes), JenvAliases: []string{alias}}, resolved)
			}
		}
		result.Jenv.Registrations = append(result.Jenv.Registrations, registration)
	}
	sort.Slice(result.Jenv.Registrations, func(i, j int) bool { return result.Jenv.Registrations[i].Alias < result.Jenv.Registrations[j].Alias })
	if value, readErr := readSelectionFile(filepath.Join(root, "version")); readErr == nil {
		result.Jenv.GlobalVersion = value
	}
	local, localFile := findLocalVersion(request.WorkingDirectory)
	if local != "" {
		result.Jenv.LocalVersion = local
		result.Jenv.LocalVersionFile = redactHome(localFile, request.Home)
	}
	result.Jenv.ShellVersion = valueOrUnknown(request.JenvShellVersion)
	result.Jenv.EffectiveVersion = firstNonEmpty(request.JenvShellVersion, local, unknownToEmpty(result.Jenv.GlobalVersion))
	if result.Jenv.EffectiveVersion == "" {
		result.Jenv.EffectiveVersion = unknown
	}
}

func discoverBuildTool(ctx context.Context, command string, args []string, request Request, deps dependencies, findings *findingCollector, parser func(string, string) BuildTool) BuildTool {
	name := command
	if command == "mvn" {
		name = "maven"
	}
	result := unknownBuildTool(name)
	discovered, err := discoverCommand(ctx, command, request)
	if err != nil {
		findings.add(strings.ToUpper(name)+"_DISCOVERY_FAILED", "A build tool could not be discovered.", []string{command})
		return result
	}
	findings.append(discovered.Findings)
	candidate := effectiveCandidate(discovered)
	if candidate == nil {
		result.State = StateNotInstalled
		return result
	}
	result.Path = candidate.Path
	result.ResolvedPath = candidate.ResolvedPath
	output, runErr := deps.runner.Run(ctx, candidate.InvocationPath(), args, executionDirectory(request), safeEnvironment(request))
	if runErr != nil {
		result.State = StateUnknown
		findings.add(strings.ToUpper(name)+"_VERSION_QUERY_FAILED", "A build-tool version query failed.", []string{candidate.Path})
		return result
	}
	parsed := parser(output, request.Home)
	parsed.Path = candidate.Path
	parsed.ResolvedPath = candidate.ResolvedPath
	if parsed.State != StateInstalled {
		findings.add(strings.ToUpper(name)+"_VERSION_OUTPUT_INVALID", "A build-tool version query returned unrecognized output.", []string{candidate.Path})
	}
	return parsed
}

func discoverGradle(ctx context.Context, request Request, current Runtime, findings *findingCollector) BuildTool {
	result := unknownBuildTool("gradle")
	discovered, err := discoverCommand(ctx, "gradle", request)
	if err != nil {
		findings.add("GRADLE_DISCOVERY_FAILED", "Gradle could not be discovered.", []string{"gradle"})
		return result
	}
	findings.append(discovered.Findings)
	candidate := effectiveCandidate(discovered)
	if candidate == nil {
		result.State = StateNotInstalled
		return result
	}
	result.Path = candidate.Path
	result.ResolvedPath = candidate.ResolvedPath
	distributionHome, version := gradleDistribution(candidate.AccessPath())
	if version == "" {
		result.State = StateUnknown
		findings.add("GRADLE_METADATA_UNAVAILABLE", "Gradle distribution metadata could not be read without executing Gradle.", []string{candidate.Path})
		return result
	}
	result.State = StateInstalled
	result.Version = version
	result.Home = redactHome(distributionHome, request.Home)
	javaHome := gradleConfiguredJavaHome(request)
	if javaHome != "" {
		result.JavaHome = redactHome(javaHome, request.Home)
		if javaVersion, _, _, releaseErr := parseReleaseFile(javaHome); releaseErr == nil {
			result.JavaVersion = javaVersion
		} else {
			findings.add("GRADLE_JAVA_HOME_INVALID", "Gradle's configured Java home does not contain readable JDK metadata.", []string{result.JavaHome})
		}
	} else if current.State == StateInstalled {
		result.JavaHome = current.Home
		result.JavaVersion = current.Version
	}
	return result
}

func gradleDistribution(executablePath string) (string, string) {
	root := filepath.Dir(filepath.Dir(executablePath))
	entries, err := os.ReadDir(filepath.Join(root, "lib"))
	if err != nil {
		return "", ""
	}
	for _, entry := range entries {
		name := entry.Name()
		for _, prefix := range []string{"gradle-core-", "gradle-runtime-api-info-"} {
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".jar") {
				version := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".jar")
				if validSelection(version) {
					return root, version
				}
			}
		}
	}
	return "", ""
}

func gradleConfiguredJavaHome(request Request) string {
	userHome := request.GradleUserHome
	if userHome == "" && request.Home != "" {
		userHome = filepath.Join(request.Home, ".gradle")
	}
	for _, path := range []string{filepath.Join(userHome, "gradle.properties"), filepath.Join(request.WorkingDirectory, "gradle.properties")} {
		if value := readGradleJavaHome(path); value != "" {
			if filepath.IsAbs(value) {
				return filepath.Clean(value)
			}
			return ""
		}
	}
	return request.JavaHome
}

func readGradleJavaHome(path string) string {
	value, err := readSmallFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(value, "\n") {
		key, property, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "org.gradle.java.home" {
			return strings.TrimSpace(property)
		}
	}
	return ""
}

func addConflictFindings(request Request, result *Result, findings *findingCollector) {
	if request.JavaHome != "" && result.Current.Home != unknown && !samePath(request.JavaHome, restoreHome(result.Current.Home, request.Home)) {
		findings.add("JAVA_HOME_RUNTIME_MISMATCH", "JAVA_HOME and the effective java executable use different JDK homes.", []string{redactHome(request.JavaHome, request.Home), result.Current.Home})
	}
	if result.Jenv.EffectiveVersion != unknown {
		registrationHome := registrationHome(result.Jenv.EffectiveVersion, result.Jenv.Registrations, request.Home)
		if registrationHome != "" && result.Current.Home != unknown && !samePath(registrationHome, restoreHome(result.Current.Home, request.Home)) {
			findings.add("JENV_RUNTIME_MISMATCH", "The effective jenv selection and java executable use different JDK homes.", []string{result.Jenv.EffectiveVersion, result.Current.Home})
		}
	}
	if result.Maven.State == StateInstalled && result.Maven.JavaVersion != unknown && result.Current.Version != unknown && !sameJavaVersion(result.Maven.JavaVersion, result.Current.Version) {
		findings.add("MAVEN_JAVA_MISMATCH", "Maven and the effective java executable use different Java versions.", []string{result.Maven.JavaVersion, result.Current.Version})
	}
	if result.Gradle.State == StateInstalled && result.Gradle.JavaVersion != unknown && result.Current.Version != unknown && !sameJavaVersion(result.Gradle.JavaVersion, result.Current.Version) {
		findings.add("GRADLE_JAVA_MISMATCH", "Gradle and the effective java executable use different Java versions.", []string{result.Gradle.JavaVersion, result.Current.Version})
	}
}

func safeEnvironment(request Request) map[string]string {
	return map[string]string{
		"PATH": strings.Join(request.PathDirectories, string(os.PathListSeparator)), "HOME": request.Home,
		"JAVA_HOME": request.JavaHome, "JENV_ROOT": request.JenvRoot, "JENV_VERSION": request.JenvShellVersion,
		"GRADLE_USER_HOME": request.GradleUserHome, "MAVEN_SKIP_RC": "1",
		"LANG": "C", "LC_ALL": "C", "NO_COLOR": "1", "TERM": "dumb",
	}
}

func discoverCommand(ctx context.Context, command string, request Request) (executable.Result, error) {
	return executable.Discover(ctx, executable.Request{Command: command, Directories: request.PathDirectories, WorkingDirectory: request.WorkingDirectory, Home: request.Home, CollectedAt: request.CollectedAt})
}

func effectiveCandidate(result executable.Result) *executable.Candidate {
	for index := range result.Candidates {
		if result.Candidates[index].Effective {
			return &result.Candidates[index]
		}
	}
	return nil
}

func readSmallFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxVersionFile {
		return "", errors.New("invalid version file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxVersionFile+1))
	if err != nil || len(data) > maxVersionFile {
		return "", errors.New("invalid version file")
	}
	return strings.TrimSpace(string(data)), nil
}

func findLocalVersion(start string) (string, string) {
	info, err := os.Stat(start)
	if err != nil || !info.IsDir() {
		return "", ""
	}
	directory := filepath.Clean(start)
	for {
		path := filepath.Join(directory, ".java-version")
		if value, readErr := readSelectionFile(path); readErr == nil && value != "" {
			return value, path
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", ""
		}
		directory = parent
	}
}

func readSelectionFile(path string) (string, error) {
	value, err := readSmallFile(path)
	if err != nil {
		return "", err
	}
	if !validSelection(value) {
		return "", errors.New("invalid Java version selection")
	}
	return value, nil
}

func validSelection(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case strings.ContainsRune("._+@-", character):
		default:
			return false
		}
	}
	return true
}

func executionDirectory(request Request) string {
	if info, err := os.Stat(request.WorkingDirectory); err == nil && info.IsDir() {
		return request.WorkingDirectory
	}
	if info, err := os.Stat(request.Home); err == nil && info.IsDir() {
		return request.Home
	}
	return string(filepath.Separator)
}

func matchJDKID(runtimeHome string, records []jdkRecord, home string) string {
	if runtimeHome == unknown {
		return ""
	}
	access := restoreHome(runtimeHome, home)
	for _, record := range records {
		if samePath(access, record.home) {
			return record.installation.ID
		}
	}
	return ""
}

func registrationHome(alias string, registrations []JenvRegistration, home string) string {
	for _, registration := range registrations {
		if registration.Alias == alias && !registration.Broken && registration.Home != unknown {
			return restoreHome(registration.Home, home)
		}
	}
	return ""
}

func managerForHome(home string, prefixes []string) Manager {
	for _, prefix := range prefixes {
		if pathWithin(home, prefix) {
			return ManagerHomebrew
		}
	}
	if home != "" {
		return ManagerSystem
	}
	return ManagerUnknown
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func samePath(left, right string) bool { return canonicalPath(left) == canonicalPath(right) }

func sameJavaVersion(left, right string) bool {
	left = strings.TrimPrefix(strings.TrimSpace(left), "v")
	right = strings.TrimPrefix(strings.TrimSpace(right), "v")
	return left == right || strings.HasPrefix(left, right+".") || strings.HasPrefix(right, left+".")
}

func containsDirectory(values []string, want string) bool {
	for _, value := range values {
		if samePath(value, want) {
			return true
		}
	}
	return false
}

func pathWithin(path, parent string) bool {
	if path == "" || parent == "" {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func redactHome(path, home string) string {
	if path == "" || home == "" {
		return filepath.Clean(path)
	}
	cleanPath, cleanHome := filepath.Clean(path), filepath.Clean(home)
	if cleanPath == cleanHome {
		return "$HOME"
	}
	if pathWithin(cleanPath, cleanHome) {
		return "$HOME" + strings.TrimPrefix(cleanPath, cleanHome)
	}
	return cleanPath
}

func restoreHome(path, home string) string {
	if path == "$HOME" {
		return home
	}
	if strings.HasPrefix(path, "$HOME"+string(filepath.Separator)) {
		return filepath.Join(home, strings.TrimPrefix(path, "$HOME"+string(filepath.Separator)))
	}
	return path
}

func jdkID(home string) string {
	sum := sha256.Sum256([]byte(home))
	return fmt.Sprintf("jdk:%x", sum[:6])
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if value != "" && !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func unknownToEmpty(value string) string {
	if value == unknown {
		return ""
	}
	return value
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return unknown
	}
	return strings.TrimSpace(value)
}

func redactUnknown(value, home string) string {
	if value == "" {
		return unknown
	}
	return redactHome(value, home)
}

func unknownRuntime() Runtime {
	return Runtime{State: StateNotInstalled, Path: unknown, ResolvedPath: unknown, Version: unknown, Home: unknown, Vendor: unknown, Architecture: inventory.ArchitectureUnknown}
}

func unknownBuildTool(name string) BuildTool {
	return BuildTool{State: StateNotInstalled, Name: name, Version: unknown, Path: unknown, ResolvedPath: unknown, Home: unknown, JavaVersion: unknown, JavaHome: unknown}
}

type findingCollector struct {
	result      *Result
	collectedAt time.Time
}

func (c *findingCollector) append(findings []inventory.Finding) {
	for _, finding := range findings {
		finding.ID = fmt.Sprintf("java-adapter-%d", len(c.result.Findings)+1)
		c.result.Findings = append(c.result.Findings, finding)
	}
}

func (c *findingCollector) add(code, message string, evidence []string) {
	source := inventory.SourceMetadata{Kind: inventory.SourceFile, Name: "Java ecosystem discovery", CollectedAt: c.collectedAt, Confidence: inventory.ConfidenceHigh}
	c.result.Findings = append(c.result.Findings, inventory.Finding{
		ID: fmt.Sprintf("java-adapter-%d", len(c.result.Findings)+1), Code: code, Severity: inventory.SeverityWarning,
		Message: message, Evidence: evidence, Confidence: inventory.ConfidenceHigh, Sources: []inventory.SourceMetadata{source},
	})
}
