package baseinstall

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

const (
	maxCatalogBytes        = 64 << 20
	maxExecutableBytes     = 4 << 20
	maxConfigurationBytes  = 64 << 10
	maxConfigurationValues = 256
	maxConfigurationValue  = 4096
	maxCatalogFormulae     = 16384
	maxTransactionFormulae = 256
)

var (
	collectorFormulaPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9@+._-]{0,127}$`)
	collectorSHA256Pattern    = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	collectorRawSHA256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

var requiredHomebrewControls = map[string]string{
	"HOMEBREW_NO_ANALYTICS":                  "1",
	"HOMEBREW_NO_ASK":                        "1",
	"HOMEBREW_NO_AUTO_UPDATE":                "1",
	"HOMEBREW_NO_ENV_HINTS":                  "1",
	"HOMEBREW_NO_INSTALL_CLEANUP":            "1",
	"HOMEBREW_NO_INSTALL_UPGRADE":            "1",
	"HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK": "1",
}

// ConfigurationSnapshot contains only the fixed Homebrew configuration
// surfaces that a future controlled adapter must assess before execution.
// File contents are parsed as data and are never evaluated by a shell.
type ConfigurationSnapshot struct {
	Environment map[string]string
	System      []byte
	Prefix      []byte
	User        []byte
}

// BottleArtifact binds an observed download size to the exact bottle digest
// selected from the locked catalog for the current macOS bottle tag.
type BottleArtifact struct {
	Formula       string
	Version       string
	BottleTag     string
	SHA256        string
	DownloadBytes int64
}

// TransactionCollectionInput contains explicit read-only snapshots. The
// collector performs no file, network, package-manager or process I/O.
type TransactionCollectionInput struct {
	ObservedAt     time.Time
	Plan           plan.Plan
	Lock           lockfile.Lock
	Inventory      inventory.Inventory
	ExecutablePath string
	ExecutableData []byte
	Configuration  ConfigurationSnapshot
	CatalogJSON    []byte
	BottleTag      string
	Artifacts      []BottleArtifact
}

// TransactionFacts are the sanitized inputs required by I21-B. They contain
// no paths, configuration values, catalog URI, commands or credentials.
type TransactionFacts struct {
	Baseline TransactionBaseline
	Actions  []ActionPreview
}

// ValidateExecutionSnapshots checks that explicit executable and
// configuration snapshots still match a sealed transaction review. It is
// pure and performs no file, process or environment discovery.
func ValidateExecutionSnapshots(
	review TransactionReview,
	executableData []byte,
	configuration ConfigurationSnapshot,
) error {
	if ValidateTransactionReview(review) != nil || len(executableData) == 0 ||
		len(executableData) > maxExecutableBytes || digestBytes(executableData) != review.Baseline.ExecutableDigest {
		return errors.New("validate Homebrew execution snapshots: executable snapshot does not match the review")
	}
	digest, unsafe, err := assessConfiguration(configuration)
	if err != nil || len(unsafe) != 0 || digest != review.Baseline.ConfigurationDigest {
		return errors.New("validate Homebrew execution snapshots: configuration snapshot does not match the review")
	}
	return nil
}

type catalogDocument []catalogFormula

type catalogFormula struct {
	Name                    string                      `json:"name"`
	FullName                string                      `json:"full_name"`
	Tap                     string                      `json:"tap"`
	Versions                catalogVersions             `json:"versions"`
	Revision                int                         `json:"revision"`
	Dependencies            []string                    `json:"dependencies"`
	RecommendedDependencies []string                    `json:"recommended_dependencies"`
	Bottle                  catalogBottle               `json:"bottle"`
	Variations              map[string]catalogVariation `json:"variations"`
	Disabled                bool                        `json:"disabled"`
}

type catalogVersions struct {
	Stable string `json:"stable"`
}

type catalogBottle struct {
	Stable catalogBottleStable `json:"stable"`
}

type catalogBottleStable struct {
	Files map[string]catalogBottleFile `json:"files"`
}

type catalogBottleFile struct {
	SHA256 string `json:"sha256"`
}

type catalogVariation struct {
	Dependencies            *[]string `json:"dependencies"`
	RecommendedDependencies *[]string `json:"recommended_dependencies"`
}

type configurationEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type configurationDigestInput struct {
	Environment []configurationEntry `json:"environment"`
	Home        string               `json:"home"`
	Temporary   string               `json:"temporary"`
	System      string               `json:"system"`
	Prefix      string               `json:"prefix"`
	User        string               `json:"user"`
}

type formulaInventoryState uint8

const (
	formulaAbsent formulaInventoryState = iota
	formulaExact
	formulaConflict
)

// CollectTransactionFacts converts exact Homebrew snapshots into the sealed
// Baseline and Action previews accepted by PrepareTransactionReview.
func CollectTransactionFacts(input TransactionCollectionInput) (TransactionFacts, error) {
	if input.ObservedAt.IsZero() {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: observed_at is required")
	}
	observedAt := input.ObservedAt.UTC()
	if err := plan.Validate(input.Plan); err != nil {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: Plan is invalid")
	}
	if observedAt.Before(input.Plan.CreatedAt) || !observedAt.Before(input.Plan.ExpiresAt) {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: Plan is not active at observed_at")
	}
	if err := lockfile.Validate(input.Lock); err != nil {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: Lock is invalid")
	}
	if _, err := inventory.Marshal(input.Inventory); err != nil ||
		input.Inventory.SchemaVersion != inventory.SchemaVersion {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: current Inventory is invalid")
	}
	if !input.Inventory.GeneratedAt.Equal(observedAt) {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: Inventory and observation times do not match")
	}
	if input.Inventory.System.OS != input.Lock.Target.OS ||
		input.Inventory.System.OSVersion != input.Lock.Target.OSVersion ||
		input.Inventory.System.Architecture != input.Lock.Target.Architecture {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: Inventory does not match the Lock target")
	}

	installation, err := observedHomebrew(input.Inventory)
	if err != nil {
		return TransactionFacts{}, err
	}
	if installation.ID != input.Plan.Environment.ActiveInstallationID ||
		installation.Version != input.Plan.Environment.ActiveVersion ||
		installation.Manager != input.Plan.Environment.ActiveManager ||
		installation.Architecture != input.Lock.Target.Architecture ||
		!path.IsAbs(input.ExecutablePath) || input.ExecutablePath != installation.Path {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: Homebrew identity, architecture or executable path changed after Plan creation")
	}
	if len(input.ExecutableData) == 0 || len(input.ExecutableData) > maxExecutableBytes {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: executable snapshot is empty or oversized")
	}

	configurationDigest, unsafeKeys, err := assessConfiguration(input.Configuration)
	if err != nil {
		return TransactionFacts{}, err
	}
	if len(unsafeKeys) != 0 {
		return TransactionFacts{}, fmt.Errorf(
			"collect Homebrew transaction: configuration is unsafe: %s",
			strings.Join(unsafeKeys, ", "),
		)
	}

	source, err := collectionSource(input.Plan, input.Lock)
	if err != nil {
		return TransactionFacts{}, err
	}
	if source.URI != "https://formulae.brew.sh/api/formula.json" {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: unsupported Homebrew catalog source")
	}
	if len(input.CatalogJSON) == 0 || len(input.CatalogJSON) > maxCatalogBytes ||
		digestBytes(input.CatalogJSON) != source.Digest {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: catalog snapshot does not match the locked source")
	}
	wantBottleTag, err := macOSBottleTag(input.Lock.Target)
	if err != nil || input.BottleTag != wantBottleTag {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: bottle tag does not match the Lock target")
	}

	catalog, err := decodeCatalog(input.CatalogJSON)
	if err != nil {
		return TransactionFacts{}, err
	}
	artifacts, err := indexArtifacts(input.Artifacts, input.BottleTag)
	if err != nil {
		return TransactionFacts{}, err
	}
	actions, usedArtifacts, err := collectActionPreviews(
		input.Plan, input.Inventory, catalog, artifacts, input.BottleTag,
	)
	if err != nil {
		return TransactionFacts{}, err
	}
	if len(usedArtifacts) != len(artifacts) {
		return TransactionFacts{}, errors.New("collect Homebrew transaction: artifact snapshot contains an unrequested formula")
	}

	facts := TransactionFacts{
		Baseline: TransactionBaseline{
			ObservedAt:          observedAt,
			InstallationID:      installation.ID,
			HomebrewVersion:     installation.Version,
			ExecutableDigest:    digestBytes(input.ExecutableData),
			ConfigurationDigest: configurationDigest,
			ConfigurationSafe:   true,
			CatalogSourceID:     source.ID,
			CatalogDigest:       source.Digest,
			CatalogSnapshotAt:   source.SnapshotAt,
		},
		Actions: cloneActionPreviews(actions),
	}
	if _, err := PrepareTransactionReview(TransactionReviewInput{
		PreparedAt: observedAt,
		Plan:       input.Plan,
		Lock:       input.Lock,
		Baseline:   facts.Baseline,
		Actions:    facts.Actions,
	}); err != nil {
		return TransactionFacts{}, fmt.Errorf("collect Homebrew transaction: collected facts are invalid: %w", err)
	}
	return facts, nil
}

func observedHomebrew(value inventory.Inventory) (inventory.Installation, error) {
	var result inventory.Installation
	found := 0
	for _, tool := range value.Tools {
		if tool.ID != "manager.homebrew" {
			continue
		}
		for _, installation := range tool.Installations {
			if installation.ActiveState == inventory.ActiveStateActive &&
				installation.Manager == "homebrew" {
				result = installation
				found++
			}
		}
	}
	if found != 1 {
		return inventory.Installation{}, errors.New("collect Homebrew transaction: exactly one active Homebrew installation is required")
	}
	return result, nil
}

func collectionSource(value plan.Plan, lock lockfile.Lock) (lockfile.Source, error) {
	sourceID := ""
	items := baseLockItems(lock)
	for _, action := range value.Actions {
		packageID := strings.TrimPrefix(action.ToolID, "homebrew.formula.")
		item, ok := items[packageID]
		if !ok || item.Implementation == nil {
			return lockfile.Source{}, errors.New("collect Homebrew transaction: Plan action has no supported locked formula")
		}
		if sourceID == "" {
			sourceID = item.Implementation.SourceID
		} else if sourceID != item.Implementation.SourceID {
			return lockfile.Source{}, errors.New("collect Homebrew transaction: Plan actions use multiple catalog sources")
		}
	}
	source, ok := transactionSource(lock, sourceID)
	if !ok {
		return lockfile.Source{}, errors.New("collect Homebrew transaction: locked Homebrew catalog source is unavailable")
	}
	return source, nil
}

func assessConfiguration(value ConfigurationSnapshot) (string, []string, error) {
	if len(value.Environment) > maxConfigurationValues {
		return "", nil, errors.New("collect Homebrew transaction: environment configuration has too many entries")
	}
	files := []struct {
		name string
		data []byte
	}{
		{name: "system", data: value.System},
		{name: "prefix", data: value.Prefix},
		{name: "user", data: value.User},
	}
	effective := make(map[string]string)
	for _, key := range []string{"HOME", "TMPDIR"} {
		entry := value.Environment[key]
		if entry == "" || len(entry) > maxConfigurationValue ||
			strings.ContainsAny(entry, ":\x00\r\n") || !path.IsAbs(entry) || path.Clean(entry) != entry {
			return "", nil, fmt.Errorf("collect Homebrew transaction: %s execution path is invalid", key)
		}
	}
	for key, entry := range value.Environment {
		if relevantHomebrewKey(key) {
			if !safeConfigurationKey(key) || len(key)+len(entry) > maxConfigurationValue ||
				strings.ContainsAny(key+entry, "\x00\r\n") {
				return "", nil, errors.New("collect Homebrew transaction: environment configuration entry is invalid")
			}
			effective[key] = entry
		}
	}
	for _, file := range files {
		entries, err := parseConfigurationFile(file.name, file.data)
		if err != nil {
			return "", nil, err
		}
		for key, entry := range entries {
			effective[key] = entry
		}
	}

	unsafe := []string{}
	for key := range effective {
		want, allowed := requiredHomebrewControls[key]
		if !allowed || effective[key] != want {
			unsafe = append(unsafe, key)
		}
	}
	for key, want := range requiredHomebrewControls {
		if effective[key] != want {
			unsafe = append(unsafe, key)
		}
	}
	sort.Strings(unsafe)
	unsafe = uniqueStrings(unsafe)

	environment := make([]configurationEntry, 0, len(value.Environment))
	for key, entry := range value.Environment {
		if relevantHomebrewKey(key) {
			environment = append(environment, configurationEntry{Name: key, Value: entry})
		}
	}
	sort.Slice(environment, func(left, right int) bool {
		return environment[left].Name < environment[right].Name
	})
	payload := configurationDigestInput{
		Environment: environment,
		Home:        value.Environment["HOME"],
		Temporary:   value.Environment["TMPDIR"],
		System:      digestBytes(value.System),
		Prefix:      digestBytes(value.Prefix),
		User:        digestBytes(value.User),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", nil, errors.New("collect Homebrew transaction: configuration digest failed")
	}
	return digestBytes(data), unsafe, nil
}

func parseConfigurationFile(name string, data []byte) (map[string]string, error) {
	if len(data) > maxConfigurationBytes {
		return nil, fmt.Errorf("collect Homebrew transaction: %s configuration file is oversized", name)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("collect Homebrew transaction: %s configuration file contains NUL", name)
	}
	result := make(map[string]string)
	if bytes.IndexByte(data, '\r') >= 0 {
		return nil, fmt.Errorf("collect Homebrew transaction: %s configuration file contains carriage return", name)
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 256 {
		return nil, fmt.Errorf("collect Homebrew transaction: %s configuration file has too many lines", name)
	}
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, entry, ok := strings.Cut(line, "=")
		if !ok || key == "" || !relevantHomebrewKey(key) || !safeConfigurationKey(key) ||
			len(key)+len(entry) > maxConfigurationValue || strings.ContainsAny(entry, "\r\n") {
			return nil, fmt.Errorf("collect Homebrew transaction: %s configuration file contains an unsupported line", name)
		}
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("collect Homebrew transaction: %s configuration file repeats key %s", name, key)
		}
		result[key] = entry
	}
	return result, nil
}

func relevantHomebrewKey(value string) bool {
	if strings.HasPrefix(value, "HOMEBREW_") || value == "SUDO_ASKPASS" {
		return true
	}
	switch value {
	case "all_proxy", "no_proxy", "ftp_proxy", "http_proxy", "https_proxy",
		"ALL_PROXY", "NO_PROXY", "FTP_PROXY", "HTTP_PROXY", "HTTPS_PROXY":
		return true
	default:
		return false
	}
}

func safeConfigurationKey(value string) bool {
	if strings.HasPrefix(value, "HOMEBREW_") || value == "SUDO_ASKPASS" {
		return configurationKeyPattern.MatchString(value)
	}
	switch value {
	case "all_proxy", "no_proxy", "ftp_proxy", "http_proxy", "https_proxy",
		"ALL_PROXY", "NO_PROXY", "FTP_PROXY", "HTTP_PROXY", "HTTPS_PROXY":
		return true
	default:
		return false
	}
}

func decodeCatalog(data []byte) (map[string]catalogFormula, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var document catalogDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("collect Homebrew transaction: decode catalog JSON: %w", err)
	}
	if _, err := decoder.Token(); err == nil || !errors.Is(err, io.EOF) {
		return nil, errors.New("collect Homebrew transaction: catalog JSON has a trailing value")
	}
	if len(document) == 0 || len(document) > maxCatalogFormulae {
		return nil, errors.New("collect Homebrew transaction: catalog formula count is invalid")
	}
	result := make(map[string]catalogFormula, len(document))
	for _, formula := range document {
		if !collectorFormulaPattern.MatchString(formula.Name) {
			return nil, errors.New("collect Homebrew transaction: catalog contains an invalid formula name")
		}
		if _, duplicate := result[formula.Name]; duplicate {
			return nil, fmt.Errorf("collect Homebrew transaction: catalog repeats formula %q", formula.Name)
		}
		result[formula.Name] = formula
	}
	return result, nil
}

func indexArtifacts(values []BottleArtifact, bottleTag string) (map[string]BottleArtifact, error) {
	if len(values) == 0 || len(values) > maxTransactionFormulae {
		return nil, errors.New("collect Homebrew transaction: bottle artifact count is invalid")
	}
	result := make(map[string]BottleArtifact, len(values))
	for _, value := range values {
		if !collectorFormulaPattern.MatchString(value.Formula) || !formulaVersionPattern.MatchString(value.Version) ||
			value.BottleTag != bottleTag || !collectorSHA256Pattern.MatchString(value.SHA256) ||
			value.DownloadBytes <= 0 {
			return nil, errors.New("collect Homebrew transaction: bottle artifact is invalid")
		}
		if _, duplicate := result[value.Formula]; duplicate {
			return nil, fmt.Errorf("collect Homebrew transaction: bottle artifact repeats formula %q", value.Formula)
		}
		result[value.Formula] = value
	}
	return result, nil
}

func collectActionPreviews(
	value plan.Plan,
	current inventory.Inventory,
	catalog map[string]catalogFormula,
	artifacts map[string]BottleArtifact,
	bottleTag string,
) ([]ActionPreview, map[string]struct{}, error) {
	result := make([]ActionPreview, 0, len(value.Actions))
	usedArtifacts := make(map[string]struct{})
	for _, action := range value.Actions {
		rootName := strings.TrimPrefix(action.ToolID, "homebrew.formula.")
		closure, err := formulaClosure(rootName, catalog, bottleTag)
		if err != nil {
			return nil, nil, err
		}
		if len(closure) > maxTransactionFormulae {
			return nil, nil, errors.New("collect Homebrew transaction: formula dependency closure is too large")
		}
		rootFormula := catalog[rootName]
		rootVersion := catalogFormulaVersion(rootFormula)
		if rootVersion != action.TargetVersion ||
			formulaInventoryStatus(current, rootName, rootVersion, current.System.Architecture) != formulaAbsent {
			return nil, nil, fmt.Errorf("collect Homebrew transaction: root formula %q changed after Plan creation", rootName)
		}
		root, err := previewFormula(rootName, rootFormula, FormulaInstallRequired, artifacts, bottleTag, usedArtifacts)
		if err != nil {
			return nil, nil, err
		}
		preview := ActionPreview{ActionID: action.ID, Root: root, Dependencies: []FormulaPreview{}}
		for _, dependencyName := range closure {
			if dependencyName == rootName {
				continue
			}
			formula := catalog[dependencyName]
			version := catalogFormulaVersion(formula)
			state := FormulaInstallRequired
			switch formulaInventoryStatus(current, dependencyName, version, current.System.Architecture) {
			case formulaExact:
				state = FormulaSatisfied
			case formulaConflict:
				return nil, nil, fmt.Errorf(
					"collect Homebrew transaction: dependency formula %q has a conflicting installed version",
					dependencyName,
				)
			}
			item, err := previewFormula(dependencyName, formula, state, artifacts, bottleTag, usedArtifacts)
			if err != nil {
				return nil, nil, err
			}
			preview.Dependencies = append(preview.Dependencies, item)
		}
		result = append(result, preview)
	}
	return result, usedArtifacts, nil
}

func formulaClosure(root string, catalog map[string]catalogFormula, bottleTag string) ([]string, error) {
	result := []string{}
	state := make(map[string]uint8)
	var visit func(string) error
	visit = func(name string) error {
		if len(state) >= maxTransactionFormulae && state[name] == 0 {
			return errors.New("collect Homebrew transaction: formula dependency closure is too large")
		}
		if state[name] == 1 {
			return fmt.Errorf("collect Homebrew transaction: formula dependency cycle at %q", name)
		}
		if state[name] == 2 {
			return nil
		}
		formula, ok := catalog[name]
		if !ok {
			return fmt.Errorf("collect Homebrew transaction: catalog is missing formula %q", name)
		}
		if (formula.FullName != "" && formula.FullName != formula.Name) ||
			formula.Tap != "homebrew/core" || formula.Revision < 0 ||
			!formulaVersionPattern.MatchString(catalogFormulaVersion(formula)) {
			return fmt.Errorf("collect Homebrew transaction: reachable formula %q is not a supported core formula", name)
		}
		if formula.Disabled {
			return fmt.Errorf("collect Homebrew transaction: formula %q is disabled", name)
		}
		state[name] = 1
		dependencies, err := formulaDependencies(formula, bottleTag)
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

func formulaDependencies(value catalogFormula, bottleTag string) ([]string, error) {
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
		if !collectorFormulaPattern.MatchString(dependency) ||
			(index > 0 && dependencies[index-1] == dependency) {
			return nil, fmt.Errorf("collect Homebrew transaction: formula %q has invalid dependencies", value.Name)
		}
	}
	return dependencies, nil
}

func previewFormula(
	name string,
	formula catalogFormula,
	state FormulaState,
	artifacts map[string]BottleArtifact,
	bottleTag string,
	used map[string]struct{},
) (FormulaPreview, error) {
	version := catalogFormulaVersion(formula)
	result := FormulaPreview{Name: name, Version: version, State: state}
	if state == FormulaSatisfied {
		return result, nil
	}
	file, ok := formula.Bottle.Stable.Files[bottleTag]
	if !ok || !collectorRawSHA256Pattern.MatchString(file.SHA256) {
		return FormulaPreview{}, fmt.Errorf("collect Homebrew transaction: formula %q has no valid bottle for %s", name, bottleTag)
	}
	artifact, ok := artifacts[name]
	if !ok || artifact.Version != version || artifact.SHA256 != "sha256:"+file.SHA256 {
		return FormulaPreview{}, fmt.Errorf("collect Homebrew transaction: formula %q has no matching bottle artifact", name)
	}
	result.DownloadBytes = artifact.DownloadBytes
	used[name] = struct{}{}
	return result, nil
}

func catalogFormulaVersion(value catalogFormula) string {
	if value.Revision == 0 {
		return value.Versions.Stable
	}
	return value.Versions.Stable + "_" + strconv.Itoa(value.Revision)
}

func formulaInventoryStatus(
	value inventory.Inventory,
	name, version string,
	architecture inventory.Architecture,
) formulaInventoryState {
	wantID := homebrewFormulaToolID(name)
	conflict := false
	for _, tool := range value.Tools {
		if tool.ID != wantID {
			continue
		}
		for _, installation := range tool.Installations {
			architectureMatches := installation.Architecture == "" ||
				installation.Architecture == inventory.ArchitectureUnknown ||
				installation.Architecture == architecture
			if !architectureMatches {
				continue
			}
			if installation.Manager == "homebrew" &&
				(installation.Version == version || installation.NormalizedVersion == version) {
				return formulaExact
			}
			conflict = true
		}
	}
	if conflict {
		return formulaConflict
	}
	return formulaAbsent
}

func homebrewFormulaToolID(name string) string {
	var result strings.Builder
	for _, character := range strings.ToLower(name) {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9', character == '-', character == '_':
			result.WriteRune(character)
		default:
			result.WriteByte('-')
		}
	}
	return "homebrew.formula." + strings.Trim(result.String(), "-_")
}

func macOSBottleTag(value lockfile.Target) (string, error) {
	if value.OS != inventory.OSMacOS {
		return "", errors.New("unsupported operating system")
	}
	major, _, _ := strings.Cut(value.OSVersion, ".")
	name := map[string]string{
		"12": "monterey",
		"13": "ventura",
		"14": "sonoma",
		"15": "sequoia",
		"26": "tahoe",
	}[major]
	if name == "" {
		return "", errors.New("unsupported macOS version")
	}
	switch value.Architecture {
	case inventory.ArchitectureARM64:
		return "arm64_" + name, nil
	case inventory.ArchitectureAMD64:
		return name, nil
	default:
		return "", errors.New("unsupported macOS architecture")
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}
