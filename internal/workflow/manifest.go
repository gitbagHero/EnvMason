package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/plan"
	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

func BuildManifest(input BuildInput) (Manifest, error) {
	if input.CreatedAt.IsZero() {
		return Manifest{}, errors.New("build workflow manifest: created_at is required")
	}
	targetNode, err := exactVersion(input.TargetNodeVersion)
	if err != nil {
		return Manifest{}, errors.New("build workflow manifest: exact stable Node.js target is required")
	}
	nodeTools, err := normalizedTargets(input.NodeTools)
	if err != nil {
		return Manifest{}, err
	}
	value := Manifest{
		SchemaVersion:     ManifestSchemaVersion,
		CreatedAt:         input.CreatedAt.UTC(),
		Executable:        false,
		Confirmable:       false,
		Summary:           manifestSummary,
		TargetNodeVersion: targetNode,
		NodeTools:         nodeTools,
		Stages:            expectedStages(),
	}
	value.ID, err = calculateManifestID(value)
	if err != nil {
		return Manifest{}, err
	}
	if err := ValidateManifest(value); err != nil {
		return Manifest{}, err
	}
	return value, nil
}

func ValidateManifest(value Manifest) error {
	if value.SchemaVersion != ManifestSchemaVersion || value.CreatedAt.IsZero() ||
		value.Executable || value.Confirmable || value.Summary != manifestSummary {
		return errors.New("validate workflow manifest: fixed identity or read-only flags are invalid")
	}
	targetNode, err := exactVersion(value.TargetNodeVersion)
	if err != nil || targetNode != value.TargetNodeVersion {
		return errors.New("validate workflow manifest: Node.js target is invalid")
	}
	targets, err := normalizedTargets(value.NodeTools)
	if err != nil || targets != value.NodeTools {
		return errors.New("validate workflow manifest: Node tools targets are invalid")
	}
	if !stagesEqual(value.Stages, expectedStages()) {
		return errors.New("validate workflow manifest: fixed stages are invalid")
	}
	expected, err := calculateManifestID(value)
	if err != nil || expected != value.ID {
		return errors.New("validate workflow manifest: content-derived ID does not match")
	}
	return nil
}

func expectedStages() []Stage {
	return []Stage{
		{
			ID: StageInstallNode, Order: 1, Risk: plan.RiskR2,
			PlanSchemaVersion: plan.ExecutableSchemaVersion,
			DependsOn:         []string{},
		},
		{
			ID: StageSetDefault, Order: 2, Risk: plan.RiskR3,
			PlanSchemaVersion: plan.HighRiskExecutableSchemaVersion,
			DependsOn:         []string{StageInstallNode},
		},
		{
			ID: StageUpdateNodeTools, Order: 3, Risk: plan.RiskR2,
			PlanSchemaVersion: plan.ExecutableSchemaVersion,
			DependsOn:         []string{StageSetDefault},
		},
	}
}

func calculateManifestID(value Manifest) (string, error) {
	copy := cloneManifest(value)
	copy.ID = ""
	data, err := json.Marshal(copy)
	if err != nil {
		return "", errors.New("calculate workflow manifest ID: encode content")
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func normalizedTargets(value NodeToolTargets) (NodeToolTargets, error) {
	result := NodeToolTargets{}
	selected := 0
	for _, target := range []struct {
		raw         string
		destination *string
	}{
		{raw: value.NPM, destination: &result.NPM},
		{raw: value.Corepack, destination: &result.Corepack},
		{raw: value.PNPM, destination: &result.PNPM},
	} {
		if target.raw == "" {
			continue
		}
		selected++
		normalized, err := exactVersion(target.raw)
		if err != nil {
			return NodeToolTargets{}, errors.New("build workflow manifest: Node tools require exact stable targets")
		}
		*target.destination = normalized
	}
	if selected == 0 {
		return NodeToolTargets{}, errors.New("build workflow manifest: at least one Node tool target is required")
	}
	return result, nil
}

func exactVersion(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return "", errors.New("exact stable version required")
	}
	parsed := versioncore.ParseSemVer(raw)
	if !parsed.Comparable || strings.Contains(parsed.Normalized, "-") ||
		strings.Contains(parsed.Normalized, "+") || parsed.Normalized != raw {
		return "", errors.New("exact stable version required")
	}
	return parsed.Normalized, nil
}

func stagesEqual(left, right []Stage) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID ||
			left[index].Order != right[index].Order ||
			left[index].Risk != right[index].Risk ||
			left[index].PlanSchemaVersion != right[index].PlanSchemaVersion ||
			!stringsEqual(left[index].DependsOn, right[index].DependsOn) {
			return false
		}
	}
	return true
}

func stringsEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cloneManifest(value Manifest) Manifest {
	result := value
	result.Stages = make([]Stage, len(value.Stages))
	for index, stage := range value.Stages {
		result.Stages[index] = stage
		result.Stages[index].DependsOn = append([]string{}, stage.DependsOn...)
	}
	return result
}

func timePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}
