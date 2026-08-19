//go:build darwin && envmason_live_i21

package baseapply

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/adapter/homebrew"
	"github.com/gitbagHero/EnvMason/internal/baseinstall"
	"github.com/gitbagHero/EnvMason/internal/discovery/macos"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/profile"
	"github.com/gitbagHero/EnvMason/internal/profileresolver"
)

const (
	liveModeEnvironment         = "ENVMASON_I21_MODE"
	liveDisposableEnvironment   = "ENVMASON_I21_DISPOSABLE"
	liveBundleEnvironment       = "ENVMASON_I21_BUNDLE"
	liveConfirmationEnvironment = "ENVMASON_I21_CONFIRM"
	liveMaximumExecutableBytes  = 4 << 20
	liveMaximumConfigBytes      = 64 << 10
	liveMaximumTokenBytes       = 16 << 10
	liveHTTPTimeout             = 30 * time.Second
)

var liveControlledEnvironment = map[string]string{
	"HOMEBREW_NO_ANALYTICS":                  "1",
	"HOMEBREW_NO_ASK":                        "1",
	"HOMEBREW_NO_AUTO_UPDATE":                "1",
	"HOMEBREW_NO_ENV_HINTS":                  "1",
	"HOMEBREW_NO_INSTALL_CLEANUP":            "1",
	"HOMEBREW_NO_INSTALL_UPGRADE":            "1",
	"HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK": "1",
}

type liveMachineSnapshot struct {
	Inventory      inventory.Inventory
	Homebrew       homebrew.Result
	ExecutableData []byte
	Configuration  baseinstall.ConfigurationSnapshot
}

type liveHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type liveClock struct {
	first bool
	last  time.Time
}

func (clock *liveClock) Now() time.Time {
	if clock.first {
		clock.first = false
		return clock.last
	}
	now := time.Now().UTC()
	if !now.After(clock.last) {
		now = clock.last.Add(time.Microsecond)
	}
	clock.last = now
	return now
}

func TestI21LiveHarness(t *testing.T) {
	mode := os.Getenv(liveModeEnvironment)
	if mode == "" {
		t.Skip("I21-F1 live harness is disabled; set the explicit mode and safety gates in a disposable VM")
	}
	if err := validateLiveEnvironment(os.Environ()); err != nil {
		t.Fatal(err)
	}
	repository, err := findLiveRepository()
	if err != nil {
		t.Fatal(err)
	}
	bundlePath := os.Getenv(liveBundleEnvironment)
	input := liveGateInput{
		GOOS: runtime.GOOS, Mode: mode,
		Disposable: os.Getenv(liveDisposableEnvironment),
		BundlePath: bundlePath, Repository: repository,
		Confirmation: os.Getenv(liveConfirmationEnvironment), Now: time.Now().UTC(),
	}
	var bundle liveBundle
	if mode == "apply" {
		data, err := readLivePrivateJSON(bundlePath, repository)
		if err != nil {
			t.Fatal(err)
		}
		bundle, err = decodeLiveBundle(data)
		if err != nil {
			t.Fatal(err)
		}
		input.Bundle = &bundle
	}
	if err := validateLiveGate(input); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Timeout: liveHTTPTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("I21 live HTTP redirects are disabled")
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	switch mode {
	case "prepare":
		if err := prepareI21Live(ctx, client, repository, bundlePath); err != nil {
			t.Fatal(err)
		}
	case "apply":
		if err := applyI21Live(ctx, client, repository, bundlePath, input.Now, bundle); err != nil {
			t.Fatal(err)
		}
	}
}

// This tagged test intentionally performs only the fixed read-only discovery
// probes. On a developer machine that already has either target formula, it
// proves the live write path is rejected before catalog or registry access.
func TestI21LiveCurrentMachineSafety(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	snapshot, err := captureLiveMachine(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateLiveEmptyBase(snapshot.Inventory); err == nil {
		t.Skip("current machine has an empty Git/CMake Homebrew base; no rejection condition to assert")
	}
}

func prepareI21Live(
	ctx context.Context,
	client liveHTTPClient,
	repository, bundlePath string,
) error {
	initial, err := captureLiveMachine(ctx, false)
	if err != nil {
		return err
	}
	if err := validateLiveEmptyBase(initial.Inventory); err != nil {
		return err
	}
	target := liveTarget(initial.Inventory)
	bottleTag, err := liveBottleTag(target)
	if err != nil {
		return err
	}
	catalogJSON, catalogAt, err := fetchLiveCatalog(ctx, client)
	if err != nil {
		return err
	}
	resolverCatalog, err := liveResolverCatalog(catalogJSON, catalogAt, target)
	if err != nil {
		return err
	}
	profileValue, err := liveBaseProfile()
	if err != nil {
		return err
	}
	lockAt := time.Now().UTC()
	lock, err := profileresolver.Resolve(profileresolver.Input{
		GeneratedAt: lockAt, Profile: profileValue, Inventory: initial.Inventory,
		Target: target, Catalog: resolverCatalog,
	})
	if err != nil {
		return fmt.Errorf("prepare I21 live Lock: %w", err)
	}
	candidate, err := plan.BuildBaseInstall(plan.BaseInstallInput{
		Lock: lock, Inventory: initial.Inventory, CreatedAt: lockAt,
	})
	if err != nil {
		return fmt.Errorf("prepare I21 live candidate Plan: %w", err)
	}
	roots, err := livePlanRoots(candidate)
	if err != nil {
		return err
	}
	specifications, err := liveArtifactSpecifications(catalogJSON, bottleTag, roots, initial.Inventory)
	if err != nil {
		return err
	}
	artifacts, err := collectLiveArtifactMetadata(ctx, client, specifications)
	if err != nil {
		return err
	}
	fresh, err := captureLiveMachine(ctx, true)
	if err != nil {
		return err
	}
	if err := validateLiveEmptyBase(fresh.Inventory); err != nil {
		return err
	}
	facts, err := baseinstall.CollectTransactionFacts(baseinstall.TransactionCollectionInput{
		ObservedAt: fresh.Inventory.GeneratedAt, Plan: candidate, Lock: lock,
		Inventory: fresh.Inventory, ExecutablePath: fresh.Homebrew.BrewPath,
		ExecutableData: fresh.ExecutableData, Configuration: fresh.Configuration,
		CatalogJSON: catalogJSON, BottleTag: bottleTag, Artifacts: artifacts,
	})
	if err != nil {
		return fmt.Errorf("prepare I21 live transaction facts: %w", err)
	}
	review, err := baseinstall.PrepareTransactionReview(baseinstall.TransactionReviewInput{
		PreparedAt: fresh.Inventory.GeneratedAt, Plan: candidate, Lock: lock,
		Baseline: facts.Baseline, Actions: facts.Actions,
	})
	if err != nil {
		return fmt.Errorf("prepare I21 live Review: %w", err)
	}
	prepared, err := Prepare(candidate, lock, review)
	if err != nil {
		return err
	}
	bundle, err := buildLiveBundle(
		review.PreparedAt, profileValue, prepared, catalogJSON, bottleTag, artifacts,
	)
	if err != nil {
		return err
	}
	bundleJSON, err := marshalLiveBundle(bundle)
	if err != nil {
		return err
	}
	if err := writeLivePrivateJSON(bundlePath, repository, bundleJSON); err != nil {
		return err
	}
	output, err := json.MarshalIndent(liveReview(prepared), "", "  ")
	if err != nil {
		return errors.New("encode I21 live review output")
	}
	fmt.Fprintf(os.Stdout, "I21-F1 prepare review (R2, read-only):\n%s\nbundle=%s\n", output, bundlePath)
	return nil
}

func applyI21Live(
	ctx context.Context,
	client liveHTTPClient,
	repository, bundlePath string,
	confirmedAt time.Time,
	bundle liveBundle,
) error {
	initial, err := captureLiveMachine(ctx, false)
	if err != nil {
		return err
	}
	if err := validateLiveEmptyBase(initial.Inventory); err != nil {
		return err
	}
	catalogJSON, _, err := fetchLiveCatalog(ctx, client)
	if err != nil {
		return err
	}
	source, found := liveCatalogSource(bundle.Prepared.Lock)
	if !found {
		return errors.New("apply I21 live: Homebrew catalog source is unavailable")
	}
	if err := validateLiveApplyEvidence(bundle, catalogJSON, nil); err != nil {
		return errors.New("apply I21 live: Homebrew catalog changed after confirmation")
	}
	roots, err := livePlanRoots(bundle.Prepared.CandidatePlan)
	if err != nil {
		return err
	}
	specifications, err := liveArtifactSpecifications(catalogJSON, bundle.BottleTag, roots, initial.Inventory)
	if err != nil {
		return err
	}
	artifacts, err := collectLiveArtifactMetadata(ctx, client, specifications)
	if err != nil {
		return err
	}
	if err := validateLiveApplyEvidence(bundle, catalogJSON, artifacts); err != nil {
		return errors.New("apply I21 live: bottle metadata changed after confirmation")
	}
	fresh, err := captureLiveMachine(ctx, true)
	if err != nil {
		return err
	}
	if err := validateLiveEmptyBase(fresh.Inventory); err != nil {
		return err
	}
	clock := &liveClock{first: true, last: fresh.Inventory.GeneratedAt}
	runner := execution.OSRunner{}
	service := Service{
		GOOS: runtime.GOOS, Now: clock.Now, Runner: runner, Verifier: runner,
		Store: execution.FileStore{Root: filepath.Join(filepath.Dir(bundlePath), "operations")},
	}
	record, err := service.Execute(ctx, bundle.Prepared, FreshSnapshots{
		Inventory: fresh.Inventory, ExecutablePath: fresh.Homebrew.BrewPath,
		ExecutableData: fresh.ExecutableData, Configuration: fresh.Configuration,
		CatalogJSON: catalogJSON, BottleTag: bundle.BottleTag, Artifacts: artifacts,
	}, Confirmation{
		Scope: ConfirmationScope, ConfirmedPlanID: bundle.Prepared.Plan.ID,
		ConfirmedReviewID: bundle.Prepared.Review.ID,
		ConfirmedAt:       confirmedAt,
	})
	if err != nil {
		return fmt.Errorf("apply I21 live transaction: %w", err)
	}
	post, err := captureLiveMachine(ctx, false)
	if err != nil {
		return err
	}
	outcome, err := Finalize(FinalizeInput{
		FinalizedAt: post.Inventory.GeneratedAt, Prepared: bundle.Prepared,
		Record: record, Inventory: post.Inventory,
	})
	if err != nil {
		return err
	}
	secondCatalog, err := liveResolverCatalog(catalogJSON, source.SnapshotAt, bundle.Prepared.Lock.Target)
	if err != nil {
		return err
	}
	secondLock, err := profileresolver.Resolve(profileresolver.Input{
		GeneratedAt: post.Inventory.GeneratedAt, Profile: bundle.Profile,
		Inventory: post.Inventory, Target: bundle.Prepared.Lock.Target, Catalog: secondCatalog,
	})
	if err != nil {
		return fmt.Errorf("verify I21 live second Lock: %w", err)
	}
	if !reflect.DeepEqual(secondLock, outcome.FinalLock) {
		return errors.New("verify I21 live second Lock: resolved state differs from finalized state")
	}
	if _, err := plan.BuildBaseInstall(plan.BaseInstallInput{
		Lock: secondLock, Inventory: post.Inventory, CreatedAt: post.Inventory.GeneratedAt,
	}); err == nil || !strings.Contains(err.Error(), "already satisfied") {
		return errors.New("verify I21 live second Plan: expected zero installation actions")
	}
	lockJSON, err := lockfile.Marshal(outcome.FinalLock)
	if err != nil {
		return err
	}
	outcomeJSON, err := json.MarshalIndent(outcome, "", "  ")
	if err != nil {
		return errors.New("encode I21 live Outcome")
	}
	outcomeJSON = append(outcomeJSON, '\n')
	suffix := strings.TrimPrefix(record.ID, "op-")
	lockPath := filepath.Join(filepath.Dir(bundlePath), "final-lock-"+suffix+".json")
	outcomePath := filepath.Join(filepath.Dir(bundlePath), "outcome-"+suffix+".json")
	if err := writeLivePrivateJSON(lockPath, repository, lockJSON); err != nil {
		return err
	}
	if err := writeLivePrivateJSON(outcomePath, repository, outcomeJSON); err != nil {
		return err
	}
	result := struct {
		OperationID string       `json:"operation_id"`
		PlanID      string       `json:"plan_id"`
		ReviewID    string       `json:"review_id"`
		FinalLockID string       `json:"final_lock_id"`
		Changes     []LockChange `json:"changes"`
		SecondPlan  string       `json:"second_plan"`
	}{record.ID, outcome.PlanID, outcome.ReviewID, outcome.FinalLock.ID, outcome.Diff.Changes, "zero_actions"}
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return errors.New("encode I21 live result output")
	}
	fmt.Fprintf(os.Stdout, "I21-F1 apply result:\n%s\nfinal_lock=%s\noutcome=%s\n", output, lockPath, outcomePath)
	return nil
}

func validateLiveApplyEvidence(
	bundle liveBundle,
	catalogJSON []byte,
	artifacts []baseinstall.BottleArtifact,
) error {
	source, found := liveCatalogSource(bundle.Prepared.Lock)
	if !found || liveDigest(catalogJSON) != source.Digest || !bytes.Equal(catalogJSON, bundle.CatalogJSON) {
		return errors.New("I21 live catalog evidence drifted")
	}
	if artifacts != nil && !reflect.DeepEqual(artifacts, bundle.Artifacts) {
		return errors.New("I21 live bottle evidence drifted")
	}
	return nil
}

func captureLiveMachine(ctx context.Context, includeExecutionSnapshots bool) (liveMachineSnapshot, error) {
	system, err := macos.Discover(ctx)
	if err != nil {
		return liveMachineSnapshot{}, fmt.Errorf("capture I21 live macOS facts: %w", err)
	}
	if len(system.Findings) != 0 {
		return liveMachineSnapshot{}, errors.New("capture I21 live macOS facts: a required read-only probe failed")
	}
	collectedAt := time.Now().UTC()
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return liveMachineSnapshot{}, errors.New("capture I21 live Homebrew facts: absolute home directory is unavailable")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return liveMachineSnapshot{}, errors.New("capture I21 live Homebrew facts: working directory is unavailable")
	}
	result, err := homebrew.Discover(ctx, homebrew.Request{
		PathDirectories:  strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)),
		WorkingDirectory: workingDirectory, Home: filepath.Clean(home),
		CollectedAt: collectedAt, ProcessArchitecture: system.System.ProcessArchitecture,
	})
	if err != nil {
		return liveMachineSnapshot{}, fmt.Errorf("capture I21 live Homebrew facts: %w", err)
	}
	if result.State != homebrew.StateInstalled || !liveVersionPattern.MatchString(result.Version) ||
		!validLiveHomebrewLocation(result, system.System.Architecture) ||
		result.Architecture != system.System.Architecture || hasUnsafeLiveFindings(result.Findings) {
		codes := make([]string, 0, len(result.Findings))
		for _, finding := range result.Findings {
			codes = append(codes, finding.Code)
		}
		sort.Strings(codes)
		return liveMachineSnapshot{}, fmt.Errorf(
			"capture I21 live Homebrew facts: one complete native active installation is required (state=%s version_known=%t system_location=%t architecture_match=%t findings=%s)",
			result.State, liveVersionPattern.MatchString(result.Version),
			validLiveHomebrewLocation(result, system.System.Architecture),
			result.Architecture == system.System.Architecture, strings.Join(codes, ","),
		)
	}
	source := inventory.SourceMetadata{
		Kind: inventory.SourcePackageManager, Name: "brew read-only adapter",
		CollectedAt: collectedAt, Confidence: inventory.ConfidenceHigh,
	}
	manager := inventory.Tool{
		ID: "manager.homebrew", DisplayName: "Homebrew", Category: inventory.CategoryEcosystem,
		Installations: []inventory.Installation{{
			ID: "homebrew:manager", Version: result.Version, Path: result.BrewPath,
			Architecture: result.Architecture, Manager: "homebrew",
			ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
			InstallReason: inventory.InstallReasonUnknown, Sources: []inventory.SourceMetadata{source},
		}},
	}
	tools := append([]inventory.Tool{}, result.Tools...)
	tools = append(tools, manager)
	sort.Slice(tools, func(left, right int) bool { return tools[left].ID < tools[right].ID })
	current := inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion, GeneratedAt: collectedAt,
		System: system.System, Tools: tools, Findings: []inventory.Finding{},
	}
	if _, err := inventory.Marshal(current); err != nil {
		return liveMachineSnapshot{}, fmt.Errorf("capture I21 live Inventory: %w", err)
	}
	snapshot := liveMachineSnapshot{Inventory: current, Homebrew: result}
	if !includeExecutionSnapshots {
		return snapshot, nil
	}
	snapshot.ExecutableData, err = readLiveRegularFile(result.BrewPath, liveMaximumExecutableBytes, false)
	if err != nil {
		return liveMachineSnapshot{}, fmt.Errorf("capture I21 live brew executable: %w", err)
	}
	snapshot.Configuration, err = captureLiveConfiguration(result, filepath.Clean(home))
	if err != nil {
		return liveMachineSnapshot{}, err
	}
	return snapshot, nil
}

func hasUnsafeLiveFindings(values []inventory.Finding) bool {
	for _, finding := range values {
		// Duplicate PATH entries can report the selected absolute executable
		// more than once. Execution never re-resolves PATH, so this finding does
		// not weaken the sealed BrewPath identity.
		if finding.Code != "EXECUTABLE_PATH_SHADOWED" {
			return true
		}
	}
	return false
}

func validLiveHomebrewLocation(result homebrew.Result, architecture inventory.Architecture) bool {
	prefix := map[inventory.Architecture]string{
		inventory.ArchitectureARM64: "/opt/homebrew",
		inventory.ArchitectureAMD64: "/usr/local",
	}[architecture]
	return prefix != "" && result.Prefix == prefix &&
		result.BrewPath == filepath.Join(prefix, "bin", "brew")
}

func captureLiveConfiguration(result homebrew.Result, home string) (baseinstall.ConfigurationSnapshot, error) {
	if err := validateLiveEnvironment(os.Environ()); err != nil {
		return baseinstall.ConfigurationSnapshot{}, err
	}
	if !filepath.IsAbs(result.Prefix) || filepath.Clean(result.Prefix) != result.Prefix {
		return baseinstall.ConfigurationSnapshot{}, errors.New("capture I21 live configuration: Homebrew prefix is invalid")
	}
	environment := make(map[string]string, len(liveControlledEnvironment)+2)
	for key, value := range liveControlledEnvironment {
		environment[key] = value
	}
	environment["HOME"] = home
	environment["TMPDIR"] = filepath.Clean(os.TempDir())
	paths := []string{
		"/etc/homebrew/brew.env",
		filepath.Join(result.Prefix, "etc", "homebrew", "brew.env"),
		filepath.Join(home, ".homebrew", "brew.env"),
	}
	values := make([][]byte, len(paths))
	for index, path := range paths {
		value, err := readLiveRegularFile(path, liveMaximumConfigBytes, true)
		if err != nil {
			return baseinstall.ConfigurationSnapshot{}, fmt.Errorf("capture I21 live configuration: %w", err)
		}
		values[index] = value
	}
	return baseinstall.ConfigurationSnapshot{
		Environment: environment, System: values[0], Prefix: values[1], User: values[2],
	}, nil
}

func validateLiveEnvironment(entries []string) error {
	for _, entry := range entries {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			return errors.New("I21 live acceptance environment contains a malformed entry")
		}
		if expected, allowed := liveControlledEnvironment[name]; allowed {
			if value != expected {
				return errors.New("I21 live acceptance environment overrides a required Homebrew safety control")
			}
			continue
		}
		if strings.HasPrefix(name, "HOMEBREW_") || name == "SUDO_ASKPASS" || name == "XDG_CONFIG_HOME" ||
			name == "all_proxy" || name == "no_proxy" || name == "ftp_proxy" || name == "http_proxy" || name == "https_proxy" ||
			name == "ALL_PROXY" || name == "NO_PROXY" || name == "FTP_PROXY" || name == "HTTP_PROXY" || name == "HTTPS_PROXY" {
			return errors.New("I21 live acceptance environment contains an unreviewed Homebrew, proxy, SUDO or XDG setting")
		}
	}
	return nil
}

func readLiveRegularFile(path string, maximum int64, optional bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if optional && errors.Is(err, os.ErrNotExist) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect fixed file %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("fixed file %q is not a bounded regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixed file %q: %w", path, err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, after) || int64(len(data)) != after.Size() {
		return nil, fmt.Errorf("fixed file %q changed while being read", path)
	}
	return data, nil
}

func fetchLiveCatalog(ctx context.Context, client liveHTTPClient) ([]byte, time.Time, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, liveCatalogURL, nil)
	if err != nil {
		return nil, time.Time{}, errors.New("fetch I21 live catalog: build fixed request")
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("fetch I21 live catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("fetch I21 live catalog: unexpected status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, liveMaximumCatalogBytes+1))
	if err != nil || len(data) == 0 || len(data) > liveMaximumCatalogBytes {
		return nil, time.Time{}, errors.New("fetch I21 live catalog: response is empty, oversized or unreadable")
	}
	if _, err := decodeLiveCatalog(data); err != nil {
		return nil, time.Time{}, err
	}
	return data, time.Now().UTC(), nil
}

func collectLiveArtifactMetadata(
	ctx context.Context,
	client liveHTTPClient,
	specifications []liveArtifactSpec,
) ([]baseinstall.BottleArtifact, error) {
	result := make([]baseinstall.BottleArtifact, 0, len(specifications))
	for _, specification := range specifications {
		size, err := fetchLiveArtifactMetadata(ctx, client, specification)
		if err != nil {
			return nil, err
		}
		artifact := specification.Artifact
		artifact.DownloadBytes = size
		result = append(result, artifact)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Formula < result[right].Formula })
	return result, nil
}

func fetchLiveArtifactMetadata(
	ctx context.Context,
	client liveHTTPClient,
	specification liveArtifactSpec,
) (int64, error) {
	challengeRequest, err := http.NewRequestWithContext(ctx, http.MethodHead, specification.URL, nil)
	if err != nil {
		return 0, errors.New("inspect I21 live bottle: build fixed HEAD request")
	}
	challengeResponse, err := client.Do(challengeRequest)
	if err != nil {
		return 0, fmt.Errorf("inspect I21 live bottle: anonymous HEAD: %w", err)
	}
	_ = challengeResponse.Body.Close()
	if challengeResponse.StatusCode != http.StatusUnauthorized ||
		parseLiveRegistryChallenge(challengeResponse.Header.Get("WWW-Authenticate"), specification.Scope) != nil {
		return 0, errors.New("inspect I21 live bottle: registry challenge is invalid")
	}
	token, err := fetchLiveRegistryToken(ctx, client, specification.Scope)
	if err != nil {
		return 0, err
	}
	headRequest, err := http.NewRequestWithContext(ctx, http.MethodHead, specification.URL, nil)
	if err != nil {
		return 0, errors.New("inspect I21 live bottle: build authorized HEAD request")
	}
	headRequest.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(headRequest)
	if err != nil {
		return 0, fmt.Errorf("inspect I21 live bottle: authorized HEAD: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength <= 0 ||
		response.Header.Get("Docker-Content-Digest") != specification.Artifact.SHA256 {
		return 0, errors.New("inspect I21 live bottle: size or digest metadata is invalid")
	}
	return response.ContentLength, nil
}

func fetchLiveRegistryToken(ctx context.Context, client liveHTTPClient, scope string) (string, error) {
	tokenURL := &url.URL{Scheme: "https", Host: "ghcr.io", Path: "/token"}
	query := tokenURL.Query()
	query.Set("service", "ghcr.io")
	query.Set("scope", scope)
	tokenURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", errors.New("fetch I21 live registry token: build fixed request")
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("fetch I21 live registry token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch I21 live registry token: unexpected status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, liveMaximumTokenBytes+1))
	if err != nil || len(data) == 0 || len(data) > liveMaximumTokenBytes {
		return "", errors.New("fetch I21 live registry token: response is empty, oversized or unreadable")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var document struct {
		Token string `json:"token"`
	}
	if err := decoder.Decode(&document); err != nil {
		return "", errors.New("fetch I21 live registry token: decode response")
	}
	if _, err := decoder.Token(); err == nil || !errors.Is(err, io.EOF) ||
		len(document.Token) == 0 || len(document.Token) > 8192 || strings.ContainsAny(document.Token, " \t\r\n") {
		return "", errors.New("fetch I21 live registry token: token is invalid")
	}
	return document.Token, nil
}

func liveBaseProfile() (profile.Profile, error) {
	buildTools, terminal := true, false
	return profile.Normalize(profile.Profile{
		SchemaVersion: profile.SchemaVersion, Name: "base-live-acceptance",
		Description: "Disposable macOS VM Base acceptance profile.",
		Modules: []profile.Module{{
			ID: profile.ModuleBase, Variant: profile.VariantMinimal,
			Options: &profile.Options{BuildTools: &buildTools, TerminalConfiguration: &terminal},
		}},
	})
}

func liveTarget(value inventory.Inventory) lockfile.Target {
	return lockfile.Target{
		OS: value.System.OS, OSVersion: value.System.OSVersion, Architecture: value.System.Architecture,
	}
}

func findLiveRepository() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", errors.New("locate I21 live repository: current directory is unavailable")
	}
	for {
		goMod, goErr := os.Lstat(filepath.Join(directory, "go.mod"))
		git, gitErr := os.Lstat(filepath.Join(directory, ".git"))
		if goErr == nil && goMod.Mode().IsRegular() && gitErr == nil && git.IsDir() {
			resolved, err := filepath.EvalSymlinks(directory)
			if err != nil {
				return "", fmt.Errorf("locate I21 live repository: %w", err)
			}
			return filepath.Clean(resolved), nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("locate I21 live repository: go.mod and .git were not found")
		}
		directory = parent
	}
}

type liveHTTPClientFunc func(*http.Request) (*http.Response, error)

func (function liveHTTPClientFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestLiveHTTPUsesFixedCatalogAndAuthenticatedHEADOnly(t *testing.T) {
	catalog := liveCatalogFixture(t)
	calls := 0
	client := liveHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodGet || request.URL.String() != liveCatalogURL ||
			request.Header.Get("Accept") != "application/json" {
			t.Fatalf("catalog request = %s %s %#v", request.Method, request.URL, request.Header)
		}
		return liveHTTPResponse(http.StatusOK, catalog, nil, int64(len(catalog))), nil
	})
	data, observedAt, err := fetchLiveCatalog(context.Background(), client)
	if err != nil || observedAt.IsZero() || !bytes.Equal(data, catalog) || calls != 1 {
		t.Fatalf("catalog = %d bytes, %v, calls=%d", len(data), err, calls)
	}

	specification := liveArtifactSpec{
		Artifact: baseinstall.BottleArtifact{
			Formula: "git", Version: "2.51.0", BottleTag: "arm64_sequoia",
			SHA256: applyDigest('a'),
		},
		URL:   "https://ghcr.io/v2/homebrew/core/git/blobs/" + applyDigest('a'),
		Scope: "repository:homebrew/core/git:pull",
	}
	calls = 0
	secret := "private-registry-token"
	client = liveHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if request.Method != http.MethodHead || request.URL.String() != specification.URL ||
				request.Header.Get("Authorization") != "" {
				t.Fatalf("anonymous request = %s %s %#v", request.Method, request.URL, request.Header)
			}
			header := make(http.Header)
			header.Set("WWW-Authenticate", `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:homebrew/core/git:pull"`)
			return liveHTTPResponse(http.StatusUnauthorized, nil, header, 0), nil
		case 2:
			if request.Method != http.MethodGet || request.URL.Scheme != "https" || request.URL.Host != "ghcr.io" ||
				request.URL.Path != "/token" || request.URL.Query().Get("service") != "ghcr.io" ||
				request.URL.Query().Get("scope") != specification.Scope || request.Header.Get("Authorization") != "" {
				t.Fatalf("token request = %s %s %#v", request.Method, request.URL, request.Header)
			}
			return liveHTTPResponse(http.StatusOK, []byte(`{"token":"`+secret+`"}`), nil, 0), nil
		case 3:
			if request.Method != http.MethodHead || request.URL.String() != specification.URL ||
				request.Header.Get("Authorization") != "Bearer "+secret {
				t.Fatalf("authorized request = %s %s %#v", request.Method, request.URL, request.Header)
			}
			header := make(http.Header)
			header.Set("Docker-Content-Digest", specification.Artifact.SHA256)
			return liveHTTPResponse(http.StatusOK, nil, header, 123456), nil
		default:
			t.Fatal("unexpected registry request")
			return nil, nil
		}
	})
	size, err := fetchLiveArtifactMetadata(context.Background(), client, specification)
	if err != nil || size != 123456 || calls != 3 {
		t.Fatalf("artifact metadata = %d, %v, calls=%d", size, err, calls)
	}
}

func TestLiveHTTPRejectsChallengeAndApplyEvidenceDrift(t *testing.T) {
	specification := liveArtifactSpec{
		Artifact: baseinstall.BottleArtifact{Formula: "git", SHA256: applyDigest('a')},
		URL:      "https://ghcr.io/v2/homebrew/core/git/blobs/" + applyDigest('a'),
		Scope:    "repository:homebrew/core/git:pull",
	}
	calls := 0
	client := liveHTTPClientFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		header := make(http.Header)
		header.Set("WWW-Authenticate", `Bearer realm="https://attacker.test/token",service="ghcr.io",scope="repository:homebrew/core/git:pull"`)
		return liveHTTPResponse(http.StatusUnauthorized, nil, header, 0), nil
	})
	if _, err := fetchLiveArtifactMetadata(context.Background(), client, specification); err == nil || calls != 1 {
		t.Fatalf("unsafe challenge = %v, calls=%d", err, calls)
	}

	bundle := validLiveBundle(t)
	if err := validateLiveApplyEvidence(bundle, bundle.CatalogJSON, bundle.Artifacts); err != nil {
		t.Fatal(err)
	}
	changedCatalog := append([]byte{}, bundle.CatalogJSON...)
	changedCatalog = append(changedCatalog, '\n')
	if err := validateLiveApplyEvidence(bundle, changedCatalog, bundle.Artifacts); err == nil {
		t.Fatal("changed catalog evidence was accepted")
	}
	changedArtifacts := append([]baseinstall.BottleArtifact{}, bundle.Artifacts...)
	changedArtifacts[0].DownloadBytes++
	if err := validateLiveApplyEvidence(bundle, bundle.CatalogJSON, changedArtifacts); err == nil {
		t.Fatal("changed bottle evidence was accepted")
	}
}

func TestLiveEnvironmentRejectsUnreviewedHomebrewAndProxySettings(t *testing.T) {
	valid := []string{
		"HOME=/Users/test", "PATH=/usr/bin:/bin", "UNRELATED=private",
		"HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1",
	}
	if err := validateLiveEnvironment(valid); err != nil {
		t.Fatalf("safe environment error = %v", err)
	}
	for _, unsafe := range []string{
		"HOMEBREW_API_DOMAIN=https://mirror.test",
		"HOMEBREW_NO_AUTO_UPDATE=0",
		"HTTPS_PROXY=http://proxy.test",
		"SUDO_ASKPASS=/private/helper",
		"XDG_CONFIG_HOME=/private/config",
	} {
		values := append(append([]string{}, valid...), unsafe)
		_, secret, _ := strings.Cut(unsafe, "=")
		if err := validateLiveEnvironment(values); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("unsafe environment was accepted or leaked its value: %q, %v", unsafe, err)
		}
	}
}

func TestLiveHomebrewLocationRequiresNativeSystemPrefix(t *testing.T) {
	for _, test := range []struct {
		architecture inventory.Architecture
		prefix       string
		brewPath     string
		valid        bool
	}{
		{inventory.ArchitectureARM64, "/opt/homebrew", "/opt/homebrew/bin/brew", true},
		{inventory.ArchitectureAMD64, "/usr/local", "/usr/local/bin/brew", true},
		{inventory.ArchitectureARM64, "/usr/local", "/usr/local/bin/brew", false},
		{inventory.ArchitectureARM64, "/Users/test/homebrew", "/Users/test/homebrew/bin/brew", false},
		{inventory.ArchitectureUnknown, "/opt/homebrew", "/opt/homebrew/bin/brew", false},
	} {
		result := homebrew.Result{Prefix: test.prefix, BrewPath: test.brewPath}
		if validLiveHomebrewLocation(result, test.architecture) != test.valid {
			t.Fatalf("location (%s, %s, %s) valid = %t", test.architecture, test.prefix, test.brewPath, !test.valid)
		}
	}
}

func liveHTTPResponse(status int, body []byte, header http.Header, length int64) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		StatusCode: status, Header: header, ContentLength: length,
		Body: io.NopCloser(bytes.NewReader(body)),
	}
}
