//go:build darwin

package baseapply

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/baseinstall"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/profile"
	"github.com/gitbagHero/EnvMason/internal/profileresolver"
)

const (
	liveMaximumFormulae     = 16384
	liveMaximumTransaction  = 256
	liveMaximumPrivateBytes = liveMaximumCatalogBytes + 8<<20
)

var (
	liveFormulaPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9@+._-]{0,127}$`)
	liveVersionPattern      = regexp.MustCompile(`^[0-9][0-9A-Za-z.+_-]{0,127}$`)
	liveRawDigestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	liveRegistryRepoPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9+._/-]{0,255}$`)
)

type liveCatalogDocument []liveCatalogFormula

type liveCatalogFormula struct {
	Name                    string                          `json:"name"`
	FullName                string                          `json:"full_name"`
	Tap                     string                          `json:"tap"`
	Versions                liveCatalogVersions             `json:"versions"`
	Revision                int                             `json:"revision"`
	Dependencies            []string                        `json:"dependencies"`
	RecommendedDependencies []string                        `json:"recommended_dependencies"`
	Bottle                  liveCatalogBottle               `json:"bottle"`
	Variations              map[string]liveCatalogVariation `json:"variations"`
	Disabled                bool                            `json:"disabled"`
}

type liveCatalogVersions struct {
	Stable string `json:"stable"`
}

type liveCatalogBottle struct {
	Stable liveCatalogBottleStable `json:"stable"`
}

type liveCatalogBottleStable struct {
	Files map[string]liveCatalogBottleFile `json:"files"`
}

type liveCatalogBottleFile struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type liveCatalogVariation struct {
	Dependencies            *[]string `json:"dependencies"`
	RecommendedDependencies *[]string `json:"recommended_dependencies"`
}

type liveArtifactSpec struct {
	Artifact baseinstall.BottleArtifact
	URL      string
	Scope    string
}

func decodeLiveCatalog(data []byte) (map[string]liveCatalogFormula, error) {
	if len(data) == 0 || len(data) > liveMaximumCatalogBytes {
		return nil, errors.New("decode I21 live catalog: snapshot is empty or oversized")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var document liveCatalogDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode I21 live catalog: %w", err)
	}
	if _, err := decoder.Token(); err == nil || !errors.Is(err, io.EOF) {
		return nil, errors.New("decode I21 live catalog: trailing JSON value")
	}
	if len(document) == 0 || len(document) > liveMaximumFormulae {
		return nil, errors.New("decode I21 live catalog: formula count is invalid")
	}
	result := make(map[string]liveCatalogFormula, len(document))
	for _, formula := range document {
		if !liveFormulaPattern.MatchString(formula.Name) {
			return nil, errors.New("decode I21 live catalog: formula name is invalid")
		}
		if _, duplicate := result[formula.Name]; duplicate {
			return nil, fmt.Errorf("decode I21 live catalog: duplicate formula %q", formula.Name)
		}
		result[formula.Name] = formula
	}
	return result, nil
}

func liveFormulaVersion(value liveCatalogFormula) string {
	if value.Revision == 0 {
		return value.Versions.Stable
	}
	return value.Versions.Stable + "_" + strconv.Itoa(value.Revision)
}

func validateReachableLiveFormula(value liveCatalogFormula) error {
	if value.FullName != "" && value.FullName != value.Name {
		return fmt.Errorf("formula %q is not an unqualified core formula", value.Name)
	}
	if value.Tap != "homebrew/core" || value.Revision < 0 || value.Disabled ||
		!liveVersionPattern.MatchString(liveFormulaVersion(value)) {
		return fmt.Errorf("formula %q is not a supported stable core formula", value.Name)
	}
	return nil
}

func liveFormulaDependencies(value liveCatalogFormula, bottleTag string) ([]string, error) {
	dependencies := append([]string{}, value.Dependencies...)
	recommended := append([]string{}, value.RecommendedDependencies...)
	if variation, ok := value.Variations[bottleTag]; ok {
		if variation.Dependencies != nil {
			dependencies = append([]string{}, (*variation.Dependencies)...)
		}
		if variation.RecommendedDependencies != nil {
			recommended = append([]string{}, (*variation.RecommendedDependencies)...)
		}
	}
	dependencies = append(dependencies, recommended...)
	sort.Strings(dependencies)
	for index, dependency := range dependencies {
		if !liveFormulaPattern.MatchString(dependency) ||
			(index > 0 && dependencies[index-1] == dependency) {
			return nil, fmt.Errorf("formula %q has invalid dependencies", value.Name)
		}
	}
	return dependencies, nil
}

func liveFormulaClosure(
	root string,
	catalog map[string]liveCatalogFormula,
	bottleTag string,
) ([]string, error) {
	result := []string{}
	state := make(map[string]uint8)
	var visit func(string) error
	visit = func(name string) error {
		if len(state) >= liveMaximumTransaction && state[name] == 0 {
			return errors.New("I21 live formula closure is too large")
		}
		if state[name] == 1 {
			return fmt.Errorf("I21 live formula dependency cycle at %q", name)
		}
		if state[name] == 2 {
			return nil
		}
		formula, ok := catalog[name]
		if !ok {
			return fmt.Errorf("I21 live catalog is missing formula %q", name)
		}
		if err := validateReachableLiveFormula(formula); err != nil {
			return err
		}
		state[name] = 1
		dependencies, err := liveFormulaDependencies(formula, bottleTag)
		if err != nil {
			return err
		}
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		result = append(result, name)
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	sort.Strings(result)
	return result, nil
}

func liveResolverCatalog(
	data []byte,
	snapshotAt time.Time,
	target lockfile.Target,
) (profileresolver.Catalog, error) {
	catalog, err := decodeLiveCatalog(data)
	if err != nil {
		return profileresolver.Catalog{}, err
	}
	if snapshotAt.IsZero() {
		return profileresolver.Catalog{}, errors.New("build I21 live resolver catalog: snapshot time is required")
	}
	if target.OS != inventory.OSMacOS ||
		(target.Architecture != inventory.ArchitectureARM64 && target.Architecture != inventory.ArchitectureAMD64) {
		return profileresolver.Catalog{}, errors.New("build I21 live resolver catalog: target is unsupported")
	}
	source := lockfile.Source{
		ID: "homebrew-core", Kind: lockfile.SourcePackageCatalog,
		URI: liveCatalogURL, SnapshotAt: snapshotAt.UTC(), Digest: liveDigest(data),
	}
	result := profileresolver.Catalog{Sources: []lockfile.Source{source}, Entries: []profileresolver.CatalogEntry{}}
	for _, root := range []struct {
		name       string
		capability string
	}{{"git", profileresolver.CapabilityBaseGit}, {"cmake", profileresolver.CapabilityBaseCMake}} {
		formula, ok := catalog[root.name]
		if !ok {
			return profileresolver.Catalog{}, fmt.Errorf("build I21 live resolver catalog: missing formula %q", root.name)
		}
		if err := validateReachableLiveFormula(formula); err != nil {
			return profileresolver.Catalog{}, fmt.Errorf("build I21 live resolver catalog: %w", err)
		}
		result.Entries = append(result.Entries, profileresolver.CatalogEntry{
			Capability: root.capability, Channel: profileresolver.ChannelStable,
			Implementation: lockfile.Implementation{
				ToolID: "homebrew.formula." + root.name, Manager: "homebrew",
				PackageKind: "formula", PackageID: root.name,
				Version: liveFormulaVersion(formula), SourceID: source.ID,
				Conditions: lockfile.Conditions{OS: inventory.OSMacOS, Architectures: []inventory.Architecture{target.Architecture}},
			},
		})
	}
	return result, nil
}

func livePlanRoots(value plan.Plan) ([]string, error) {
	if err := plan.Validate(value); err != nil {
		return nil, errors.New("collect I21 live roots: Plan is invalid")
	}
	result := make([]string, 0, len(value.Actions))
	for _, action := range value.Actions {
		name := strings.TrimPrefix(action.ToolID, "homebrew.formula.")
		if (name != "git" && name != "cmake") || action.ToolID != "homebrew.formula."+name ||
			action.Operation != "install" || action.Adapter != "homebrew" {
			return nil, errors.New("collect I21 live roots: Plan contains an unsupported action")
		}
		result = append(result, name)
	}
	sort.Strings(result)
	if len(result) == 0 || len(result) > 2 || (len(result) == 2 && result[0] == result[1]) {
		return nil, errors.New("collect I21 live roots: Plan action set is invalid")
	}
	return result, nil
}

func liveArtifactSpecifications(
	data []byte,
	bottleTag string,
	roots []string,
	current inventory.Inventory,
) ([]liveArtifactSpec, error) {
	catalog, err := decodeLiveCatalog(data)
	if err != nil {
		return nil, err
	}
	rootSet := make(map[string]struct{}, len(roots))
	formulae := make(map[string]struct{})
	for _, root := range roots {
		if root != "git" && root != "cmake" {
			return nil, errors.New("collect I21 live artifacts: unsupported root")
		}
		if _, duplicate := rootSet[root]; duplicate {
			return nil, errors.New("collect I21 live artifacts: duplicate root")
		}
		rootSet[root] = struct{}{}
		closure, err := liveFormulaClosure(root, catalog, bottleTag)
		if err != nil {
			return nil, err
		}
		for _, name := range closure {
			formulae[name] = struct{}{}
		}
	}
	if len(formulae) == 0 || len(formulae) > liveMaximumTransaction {
		return nil, errors.New("collect I21 live artifacts: formula closure is invalid")
	}
	names := make([]string, 0, len(formulae))
	for name := range formulae {
		names = append(names, name)
	}
	sort.Strings(names)
	result := []liveArtifactSpec{}
	for _, name := range names {
		formula := catalog[name]
		version := liveFormulaVersion(formula)
		state := liveFormulaInventoryStatus(current, name, version)
		_, root := rootSet[name]
		if root && state != liveFormulaAbsent {
			return nil, fmt.Errorf("collect I21 live artifacts: root formula %q is no longer absent", name)
		}
		if state == liveFormulaConflict {
			return nil, fmt.Errorf("collect I21 live artifacts: formula %q has a conflicting installation", name)
		}
		if state == liveFormulaExact {
			continue
		}
		file, ok := formula.Bottle.Stable.Files[bottleTag]
		if !ok || !liveRawDigestPattern.MatchString(file.SHA256) {
			return nil, fmt.Errorf("collect I21 live artifacts: formula %q has no valid %s bottle", name, bottleTag)
		}
		digest := "sha256:" + file.SHA256
		scope, err := validateLiveBottleURL(file.URL, name, digest)
		if err != nil {
			return nil, fmt.Errorf("collect I21 live artifacts: formula %q: %w", name, err)
		}
		result = append(result, liveArtifactSpec{
			Artifact: baseinstall.BottleArtifact{Formula: name, Version: version, BottleTag: bottleTag, SHA256: digest},
			URL:      file.URL, Scope: scope,
		})
	}
	return result, nil
}

type liveFormulaState uint8

const (
	liveFormulaAbsent liveFormulaState = iota
	liveFormulaExact
	liveFormulaConflict
)

func liveFormulaInventoryStatus(value inventory.Inventory, name, version string) liveFormulaState {
	wantID := "homebrew.formula." + strings.Trim(strings.NewReplacer("@", "-", "+", "-", ".", "-").Replace(name), "-_")
	conflict := false
	for _, tool := range value.Tools {
		if tool.ID != wantID {
			continue
		}
		for _, installation := range tool.Installations {
			architectureMatches := installation.Architecture == "" ||
				installation.Architecture == inventory.ArchitectureUnknown ||
				installation.Architecture == value.System.Architecture
			if !architectureMatches {
				continue
			}
			if installation.Manager == "homebrew" &&
				(installation.Version == version || installation.NormalizedVersion == version) {
				return liveFormulaExact
			}
			conflict = true
		}
	}
	if conflict {
		return liveFormulaConflict
	}
	return liveFormulaAbsent
}

func validateLiveBottleURL(raw, formula, digest string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "ghcr.io" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" || !liveDigestPattern.MatchString(digest) {
		return "", errors.New("bottle URL is outside the fixed GHCR boundary")
	}
	repository := "homebrew/core/" + strings.ReplaceAll(formula, "@", "/")
	if !liveRegistryRepoPattern.MatchString(repository) ||
		parsed.Path != "/v2/"+repository+"/blobs/"+digest {
		return "", errors.New("bottle URL does not bind the formula digest")
	}
	return "repository:" + repository + ":pull", nil
}

func parseLiveRegistryChallenge(value, expectedScope string) error {
	if !strings.HasPrefix(value, "Bearer ") {
		return errors.New("GHCR challenge is not Bearer")
	}
	parameters, err := parseLiveChallengeParameters(strings.TrimPrefix(value, "Bearer "))
	if err != nil || len(parameters) != 3 ||
		parameters["realm"] != "https://ghcr.io/token" ||
		parameters["service"] != "ghcr.io" || parameters["scope"] != expectedScope {
		return errors.New("GHCR challenge escaped the fixed token boundary")
	}
	return nil
}

func parseLiveChallengeParameters(value string) (map[string]string, error) {
	result := make(map[string]string)
	for value != "" {
		value = strings.TrimLeft(value, " ")
		key, remainder, found := strings.Cut(value, "=")
		if !found || key == "" || strings.TrimSpace(key) != key || !strings.HasPrefix(remainder, `"`) {
			return nil, errors.New("invalid challenge parameter")
		}
		remainder = remainder[1:]
		end := strings.IndexByte(remainder, '"')
		if end < 0 || strings.ContainsAny(remainder[:end], "\\\r\n") {
			return nil, errors.New("invalid challenge parameter value")
		}
		if _, duplicate := result[key]; duplicate {
			return nil, errors.New("duplicate challenge parameter")
		}
		result[key] = remainder[:end]
		value = remainder[end+1:]
		if value == "" {
			break
		}
		if !strings.HasPrefix(value, ",") {
			return nil, errors.New("invalid challenge separator")
		}
		value = value[1:]
	}
	return result, nil
}

func ensureLivePrivateDestination(path, repository string, createDirectory bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Ext(path) != ".json" ||
		!filepath.IsAbs(repository) || filepath.Clean(repository) != repository {
		return errors.New("I21 live private destination must be one clean absolute JSON path")
	}
	parent := filepath.Dir(path)
	created := false
	if createDirectory {
		if _, err := os.Lstat(parent); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(parent, 0o700); err != nil {
				return fmt.Errorf("create I21 live private directory: %w", err)
			}
			created = true
		} else if err != nil {
			return fmt.Errorf("inspect I21 live private directory: %w", err)
		}
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("inspect I21 live private directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("I21 live private parent must be a real directory")
	}
	if created {
		if err := os.Chmod(parent, 0o700); err != nil {
			return fmt.Errorf("protect I21 live private directory: %w", err)
		}
		info, err = os.Lstat(parent)
		if err != nil {
			return fmt.Errorf("reinspect I21 live private directory: %w", err)
		}
	}
	if info.Mode().Perm() != 0o700 {
		return errors.New("I21 live private parent must already have mode 0700")
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("resolve I21 live private directory: %w", err)
	}
	resolvedRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		return fmt.Errorf("resolve I21 live repository: %w", err)
	}
	resolvedPath := filepath.Join(resolvedParent, filepath.Base(path))
	if resolvedPath == resolvedRepository || strings.HasPrefix(resolvedPath, resolvedRepository+string(filepath.Separator)) {
		return errors.New("I21 live private destination resolves inside the repository")
	}
	return nil
}

func writeLivePrivateJSON(path, repository string, data []byte) error {
	if len(data) == 0 || len(data) > liveMaximumPrivateBytes {
		return errors.New("write I21 live private JSON: content is empty or oversized")
	}
	if err := ensureLivePrivateDestination(path, repository, true); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create I21 live private JSON without replacement: %w", err)
	}
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("protect I21 live private JSON: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write I21 live private JSON: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync I21 live private JSON: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close I21 live private JSON: %w", err)
	}
	remove = false
	return nil
}

func readLivePrivateJSON(path, repository string) ([]byte, error) {
	if err := ensureLivePrivateDestination(path, repository, false); err != nil {
		return nil, err
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect I21 live private JSON: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Mode().Perm() != 0o600 ||
		before.Size() <= 0 || before.Size() > liveMaximumPrivateBytes {
		return nil, errors.New("I21 live private JSON must be a bounded private regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read I21 live private JSON: %w", err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || int64(len(data)) != after.Size() {
		return nil, errors.New("I21 live private JSON changed while being read")
	}
	return data, nil
}

func TestLiveCatalogClosureAndGHCRBoundary(t *testing.T) {
	data := liveCatalogFixture(t)
	catalog, err := decodeLiveCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := liveFormulaClosure("git", catalog, "arm64_sequoia")
	if err != nil || !strings.EqualFold(strings.Join(closure, ","), "gettext,git") {
		t.Fatalf("closure = %#v, %v", closure, err)
	}
	current := applyInventory(applyTestTime, false)
	specifications, err := liveArtifactSpecifications(data, "arm64_sequoia", []string{"git", "cmake"}, current)
	if err != nil || len(specifications) != 3 {
		t.Fatalf("artifact specifications = %#v, %v", specifications, err)
	}
	for _, specification := range specifications {
		challenge := `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="` + specification.Scope + `"`
		if err := parseLiveRegistryChallenge(challenge, specification.Scope); err != nil {
			t.Fatalf("valid challenge error = %v", err)
		}
	}
	badURLs := []string{
		"http://ghcr.io/v2/homebrew/core/git/blobs/" + applyDigest('a'),
		"https://example.com/v2/homebrew/core/git/blobs/" + applyDigest('a'),
		"https://ghcr.io/v2/homebrew/core/cmake/blobs/" + applyDigest('a'),
		"https://ghcr.io/v2/homebrew/core/git/blobs/" + applyDigest('b'),
	}
	for _, raw := range badURLs {
		if _, err := validateLiveBottleURL(raw, "git", applyDigest('a')); err == nil {
			t.Fatalf("unsafe bottle URL accepted: %s", raw)
		}
	}
	if err := parseLiveRegistryChallenge(
		`Bearer realm="https://attacker.test/token",service="ghcr.io",scope="repository:homebrew/core/git:pull"`,
		"repository:homebrew/core/git:pull",
	); err == nil {
		t.Fatal("unsafe GHCR realm accepted")
	}
}

func TestLiveHarnessSimulationBuildsBoundReviewBundle(t *testing.T) {
	catalogJSON := liveCatalogFixture(t)
	initialAt := applyTestTime.Add(-3 * time.Minute)
	initial := applyInventory(initialAt, false)
	target := lockfile.Target{
		OS: initial.System.OS, OSVersion: initial.System.OSVersion, Architecture: initial.System.Architecture,
	}
	resolverCatalog, err := liveResolverCatalog(catalogJSON, initialAt.Add(time.Minute), target)
	if err != nil {
		t.Fatal(err)
	}
	buildTools, terminal := true, false
	profileValue, err := profile.Normalize(profile.Profile{
		SchemaVersion: profile.SchemaVersion, Name: "base-live-acceptance",
		Modules: []profile.Module{{
			ID: profile.ModuleBase, Variant: profile.VariantMinimal,
			Options: &profile.Options{BuildTools: &buildTools, TerminalConfiguration: &terminal},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lockAt := initialAt.Add(2 * time.Minute)
	lockValue, err := profileresolver.Resolve(profileresolver.Input{
		GeneratedAt: lockAt, Profile: profileValue, Inventory: initial,
		Target: target, Catalog: resolverCatalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := plan.BuildBaseInstall(plan.BaseInstallInput{
		Lock: lockValue, Inventory: initial, CreatedAt: lockAt,
	})
	if err != nil || len(candidate.Actions) != 2 {
		t.Fatalf("candidate Plan = %#v, %v", candidate, err)
	}
	roots, err := livePlanRoots(candidate)
	if err != nil {
		t.Fatal(err)
	}
	specifications, err := liveArtifactSpecifications(catalogJSON, "arm64_sequoia", roots, initial)
	if err != nil || len(specifications) != 3 {
		t.Fatalf("artifact specifications = %#v, %v", specifications, err)
	}
	artifacts := make([]baseinstall.BottleArtifact, 0, len(specifications))
	for index, specification := range specifications {
		artifact := specification.Artifact
		artifact.DownloadBytes = int64(index + 1)
		artifacts = append(artifacts, artifact)
	}
	reviewAt := applyTestTime
	fresh := applyInventory(reviewAt, false)
	configuration := baseinstall.ConfigurationSnapshot{Environment: map[string]string{
		"HOME": "/Users/envmason", "TMPDIR": "/private/tmp/envmason",
		"HOMEBREW_NO_ANALYTICS": "1", "HOMEBREW_NO_ASK": "1", "HOMEBREW_NO_AUTO_UPDATE": "1",
		"HOMEBREW_NO_ENV_HINTS": "1", "HOMEBREW_NO_INSTALL_CLEANUP": "1",
		"HOMEBREW_NO_INSTALL_UPGRADE": "1", "HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK": "1",
	}}
	facts, err := baseinstall.CollectTransactionFacts(baseinstall.TransactionCollectionInput{
		ObservedAt: reviewAt, Plan: candidate, Lock: lockValue, Inventory: fresh,
		ExecutablePath: "/opt/homebrew/bin/brew", ExecutableData: []byte("fixed brew executable"),
		Configuration: configuration, CatalogJSON: catalogJSON,
		BottleTag: "arm64_sequoia", Artifacts: artifacts,
	})
	if err != nil {
		t.Fatal(err)
	}
	review, err := baseinstall.PrepareTransactionReview(baseinstall.TransactionReviewInput{
		PreparedAt: reviewAt, Plan: candidate, Lock: lockValue,
		Baseline: facts.Baseline, Actions: facts.Actions,
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(candidate, lockValue, review)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := buildLiveBundle(reviewAt, profileValue, prepared, catalogJSON, "arm64_sequoia", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Prepared.Plan.ID == bundle.Prepared.CandidatePlan.ID ||
		bundle.Prepared.Review.Summary.InstallRequired != len(bundle.Artifacts) ||
		liveConfirmationToken(bundle.Prepared) == "" {
		t.Fatalf("simulated live bundle = %#v", bundle)
	}
}

func TestLivePrivateJSONRejectsReplacementPermissionsAndSymlinks(t *testing.T) {
	repository := t.TempDir()
	root := t.TempDir()
	path := filepath.Join(root, "evidence", "bundle.json")
	data := []byte("{\"safe\":true}\n")
	if err := writeLivePrivateJSON(path, repository, data); err != nil {
		t.Fatal(err)
	}
	if err := writeLivePrivateJSON(path, repository, data); err == nil {
		t.Fatal("private evidence was replaced")
	}
	read, err := readLivePrivateJSON(path, repository)
	if err != nil || !bytes.Equal(read, data) {
		t.Fatalf("private evidence = %q, %v", read, err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readLivePrivateJSON(path, repository); err == nil {
		t.Fatal("world-readable private evidence was accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repository, "target.json"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := readLivePrivateJSON(path, repository); err == nil {
		t.Fatal("symlink private evidence was accepted")
	}
	inside := filepath.Join(repository, "inside.json")
	if err := writeLivePrivateJSON(inside, repository, data); err == nil {
		t.Fatal("repository-local private evidence was accepted")
	}
	public := t.TempDir()
	if err := os.Chmod(public, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLivePrivateJSON(filepath.Join(public, "bundle.json"), repository, data); err == nil {
		t.Fatal("existing public parent directory was accepted")
	}
	info, err := os.Lstat(public)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("public parent permissions were mutated: %#v, %v", info, err)
	}
}

func liveCatalogFixture(t *testing.T) []byte {
	t.Helper()
	digest := func(character byte) string { return strings.Repeat(string(character), 64) }
	file := func(formula string, character byte) liveCatalogBottleFile {
		raw := digest(character)
		return liveCatalogBottleFile{
			URL:    "https://ghcr.io/v2/homebrew/core/" + strings.ReplaceAll(formula, "@", "/") + "/blobs/sha256:" + raw,
			SHA256: raw,
		}
	}
	formula := func(name, version string, dependencies []string, character byte) liveCatalogFormula {
		return liveCatalogFormula{
			Name: name, FullName: name, Tap: "homebrew/core", Versions: liveCatalogVersions{Stable: version},
			Dependencies: dependencies, RecommendedDependencies: []string{}, Variations: map[string]liveCatalogVariation{},
			Bottle: liveCatalogBottle{Stable: liveCatalogBottleStable{Files: map[string]liveCatalogBottleFile{
				"arm64_sequoia": file(name, character),
			}}},
		}
	}
	document := liveCatalogDocument{
		formula("git", "2.51.0", []string{"gettext"}, 'a'),
		formula("gettext", "0.26", nil, 'b'),
		formula("cmake", "4.0.3", nil, 'c'),
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
