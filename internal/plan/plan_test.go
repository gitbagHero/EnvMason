package plan

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/assessment"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/report"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

func TestBuildIsDeterministicImmutableAndNonExecutable(t *testing.T) {
	t.Parallel()
	input := buildFixture()
	first, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) || first.ID != second.ID {
		t.Fatal("fixed input produced different plans")
	}
	if first.Executable || first.ExpiresAt.Sub(first.CreatedAt) != DefaultTTL || first.Actions[0].Risk != RiskR2 || !first.Actions[0].Confirmation.Required {
		t.Fatalf("plan safety contract = %#v", first)
	}
	for _, forbidden := range []string{`"command"`, `"args"`, `"shell"`, `"executable": true`} {
		if bytes.Contains(firstJSON, []byte(forbidden)) {
			t.Errorf("plan exposes forbidden execution field %q: %s", forbidden, firstJSON)
		}
	}

	mutated := first
	mutated.Actions = append([]Action{}, first.Actions...)
	mutated.Actions[0].TargetVersion = "v25.0.0"
	if err := Validate(mutated); err == nil || !strings.Contains(err.Error(), "Plan ID") {
		t.Fatalf("mutated plan validation = %v", err)
	}
	newID, err := planID(mutated)
	if err != nil || newID == first.ID {
		t.Fatalf("mutated plan ID = %s, %v", newID, err)
	}
}

func TestBuildDigestsResolvedPolicySemantics(t *testing.T) {
	t.Parallel()
	implicit := buildFixture()
	implicit.Policy = assessment.Policy{SchemaVersion: assessment.PolicySchemaVersion, Tools: map[string]assessment.ToolPolicy{}}
	explicit := buildFixture()
	first, err := Build(implicit)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if first.PolicyDigest != second.PolicyDigest || first.ID != second.ID {
		t.Fatalf("semantic defaults changed plan identity: %s/%s and %s/%s", first.PolicyDigest, second.PolicyDigest, first.ID, second.ID)
	}
}

func TestValidateRejectsInvalidRiskVerifierAndDependencies(t *testing.T) {
	t.Parallel()
	base, err := Build(buildFixture())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(*Plan)
		want string
	}{
		{name: "unknown risk", edit: func(value *Plan) { value.Actions[0].Risk = "RX" }, want: "unknown risk"},
		{name: "risk downgrade", edit: func(value *Plan) { value.Actions[0].Risk = RiskR1 }, want: "lower than R2"},
		{name: "missing verifier", edit: func(value *Plan) { value.Actions[0].Verifications = nil }, want: "verifications"},
		{name: "missing dependency", edit: func(value *Plan) { value.Actions[0].Dependencies = []string{"missing"} }, want: "unknown dependency"},
		{name: "duplicate dependency", edit: func(value *Plan) { value.Actions[0].Dependencies = []string{"missing", "missing"} }, want: "unique"},
		{name: "invalid target", edit: func(value *Plan) { value.Actions[0].TargetVersion = "latest" }, want: "unsupported"},
		{name: "missing recovery", edit: func(value *Plan) { value.Actions[0].Recovery.Summary = "" }, want: "recovery"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			value := clonePlan(base)
			test.edit(&value)
			value.ID, _ = planID(value)
			if err := Validate(value); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsDependencyCycle(t *testing.T) {
	t.Parallel()
	value, err := Build(buildFixture())
	if err != nil {
		t.Fatal(err)
	}
	second := value.Actions[0]
	second.ID = "verify-installation"
	second.Dependencies = []string{value.Actions[0].ID}
	value.Actions[0].Dependencies = []string{second.ID}
	value.Actions = append(value.Actions, second)
	value.ID, _ = planID(value)
	if err := Validate(value); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle validation = %v", err)
	}
}

func TestPlanJSONUsesVersionedSchemaAndStrictDecode(t *testing.T) {
	t.Parallel()
	value, err := Build(buildFixture())
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSON(data); err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil || decoded.ID != value.ID {
		t.Fatalf("Decode = %#v, %v", decoded, err)
	}
	unknown := bytes.Replace(data, []byte(`"summary":`), []byte(`"unknown":true,"summary":`), 1)
	if err := ValidateJSON(unknown); err == nil {
		t.Fatal("schema accepted an unknown field")
	}
	wrongVersion := bytes.Replace(data, []byte(`"schema_version": "0.1.0"`), []byte(`"schema_version": "9.9.9"`), 1)
	if err := ValidateJSON(wrongVersion); err == nil {
		t.Fatal("schema accepted an unknown version")
	}
	for name, invalid := range map[string][]byte{
		"executable": bytes.Replace(data, []byte(`"executable": false`), []byte(`"executable": true`), 1),
		"risk":       bytes.Replace(data, []byte(`"risk": "R2"`), []byte(`"risk": "R1"`), 1),
		"elevation":  bytes.Replace(data, []byte(`"elevation_required": false`), []byte(`"elevation_required": true`), 1),
	} {
		if err := ValidateJSON(invalid); err == nil {
			t.Errorf("schema accepted unsafe %s mutation", name)
		}
	}
}

func TestBuildRequiresNVMAndMatchingActiveCandidate(t *testing.T) {
	t.Parallel()
	input := buildFixture()
	input.Inventory.Tools[0].Installations[0].Manager = "homebrew"
	if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "NVM") {
		t.Fatalf("without NVM error = %v", err)
	}
	input = buildFixture()
	input.Versions.Node.LatestLTSFreshness = versiondata.FreshnessStale
	if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "no eligible") {
		t.Fatalf("stale candidate error = %v", err)
	}
}

func TestBuildAndValidateEnforceThirtyMinuteTTL(t *testing.T) {
	t.Parallel()
	input := buildFixture()
	input.TTL = time.Hour
	if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "30m0s") {
		t.Fatalf("custom TTL error = %v", err)
	}
	value, err := Build(buildFixture())
	if err != nil {
		t.Fatal(err)
	}
	value.ExpiresAt = value.CreatedAt.Add(time.Hour)
	value.ID, _ = planID(value)
	if err := Validate(value); err == nil || !strings.Contains(err.Error(), "30m0s") {
		t.Fatalf("mutated TTL validation = %v", err)
	}
}

func TestBuildRequiresFreshOfficialEvidenceForPinnedTarget(t *testing.T) {
	t.Parallel()
	input := buildFixture()
	input.Policy.Tools["runtime.node"] = assessment.ToolPolicy{Channel: assessment.ChannelLTS, Pin: "23.1.0"}
	if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("unverified Pin error = %v", err)
	}
	input.Versions.Node.AvailableVersions = append(input.Versions.Node.AvailableVersions, "v23.1.0")
	value, err := Build(input)
	if err != nil || value.Actions[0].TargetVersion != "23.1.0" {
		t.Fatalf("verified Pin plan = %#v, %v", value, err)
	}
}

func TestBuildExecutableBindsNVMAndDefaultAliasDigests(t *testing.T) {
	t.Parallel()
	input := buildFixture()
	input.Policy.Tools["runtime.node"] = assessment.ToolPolicy{Channel: assessment.ChannelLTS, Pin: "24.2.0"}
	script := "sha256:" + strings.Repeat("a", 64)
	alias := "sha256:" + strings.Repeat("b", 64)
	value, err := BuildExecutable(input, script, alias)
	if err != nil {
		t.Fatal(err)
	}
	if value.SchemaVersion != ExecutableSchemaVersion || !value.Executable || value.Actions[0].Risk != RiskR2 {
		t.Fatalf("executable Plan = %#v", value)
	}
	summary, err := Render(value, FormatSummary)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{script, alias, "exact plan-level confirmation"} {
		if !strings.Contains(string(summary), expected) {
			t.Fatalf("summary does not expose %q:\n%s", expected, summary)
		}
	}
	mutated := clonePlan(value)
	mutated.Actions[0].Preconditions[len(mutated.Actions[0].Preconditions)-1].Expected = "sha256:" + strings.Repeat("c", 64)
	if err := Validate(mutated); err == nil || !strings.Contains(err.Error(), "Plan ID") {
		t.Fatalf("mutated precondition = %v", err)
	}
	if _, err := BuildExecutable(input, "bad", alias); err == nil {
		t.Fatal("invalid script digest was accepted")
	}
}

func TestGenerateRequiresFreshEligibleRecommendationAndOnlyRenders(t *testing.T) {
	t.Parallel()
	input := buildFixture()
	result := report.AssessmentResult{Inventory: input.Inventory, Policy: input.Policy, Versions: versiondata.Result{Node: versiondata.NodeData{LatestLTS: "v24.2.0", LatestLTSFreshness: versiondata.FreshnessFresh}}}
	called := 0
	assess := func(context.Context, report.Options) (report.AssessmentResult, error) {
		called++
		return result, nil
	}
	data, err := generate(context.Background(), Options{ToolID: "runtime.node", Format: FormatJSON, Online: true}, func() time.Time { return input.CreatedAt }, assess)
	if err != nil || called != 1 || !bytes.Contains(data, []byte(`"executable": false`)) || !bytes.Contains(data, []byte(`"risk": "R2"`)) {
		t.Fatalf("generate = called %d, %v\n%s", called, err, data)
	}
	result.Versions.Node.LatestLTSFreshness = versiondata.FreshnessStale
	if _, err := generate(context.Background(), Options{ToolID: "runtime.node", Online: true}, time.Now, assess); err == nil || !strings.Contains(err.Error(), "no eligible") {
		t.Fatalf("stale generation error = %v", err)
	}
	if _, err := generate(context.Background(), Options{ToolID: "runtime.node"}, time.Now, assess); err == nil || called != 2 {
		t.Fatalf("offline validation/calls = %v/%d", err, called)
	}
}

func TestRenderSummaryMakesPreviewBoundaryVisible(t *testing.T) {
	t.Parallel()
	value, err := Build(buildFixture())
	if err != nil {
		t.Fatal(err)
	}
	data, err := Render(value, FormatSummary)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"EnvMason plan preview", "Executable: false", "risk=R2", "confirmation=plan", "cannot execute or modify"} {
		if !strings.Contains(string(data), expected) {
			t.Errorf("summary missing %q:\n%s", expected, data)
		}
	}
}

func buildFixture() BuildInput {
	createdAt := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	policy := assessment.DefaultPolicy()
	return BuildInput{
		Inventory: inventory.Inventory{
			SchemaVersion: inventory.SchemaVersion, GeneratedAt: createdAt,
			System: inventory.System{OS: inventory.OSMacOS, OSVersion: "26.0", Architecture: inventory.ArchitectureARM64},
			Tools: []inventory.Tool{{ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime, Installations: []inventory.Installation{{
				ID: "node-nvm-22", Version: "v22.0.0", Path: "$HOME/.nvm/versions/node/v22.0.0/bin/node", Manager: "nvm",
				ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
			}}}},
		},
		Policy: policy,
		Versions: versiondata.Result{Node: versiondata.NodeData{
			LatestLTS: "v24.2.0", LatestLTSFreshness: versiondata.FreshnessFresh,
			AvailableVersions: []string{"v22.0.0", "v24.2.0"}, ReleaseIndexFreshness: versiondata.FreshnessFresh,
		}},
		CreatedAt: createdAt, TTL: DefaultTTL,
	}
}

func clonePlan(value Plan) Plan {
	result := value
	result.Actions = append([]Action{}, value.Actions...)
	for index := range result.Actions {
		result.Actions[index].Dependencies = append([]string{}, value.Actions[index].Dependencies...)
		result.Actions[index].Preconditions = append([]Check{}, value.Actions[index].Preconditions...)
		result.Actions[index].Verifications = append([]Check{}, value.Actions[index].Verifications...)
	}
	return result
}
