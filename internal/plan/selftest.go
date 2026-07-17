package plan

import (
	"errors"
	"fmt"
	"time"
)

// SelfTestInput contains the stable local facts needed to bind the harmless
// I14 executor self-test to one environment snapshot.
type SelfTestInput struct {
	CreatedAt    time.Time
	OS           string
	OSVersion    string
	Architecture string
}

// BuildSelfTest creates an executable R1 Plan for the single built-in I14
// self-test. It never embeds a command, executable path, argument or shell.
func BuildSelfTest(input SelfTestInput) (Plan, error) {
	if input.CreatedAt.IsZero() || input.OS == "" || input.OSVersion == "" || input.Architecture == "" {
		return Plan{}, errors.New("build self-test plan: created_at and environment are required")
	}
	environment := EnvironmentSummary{
		OS: input.OS, OSVersion: input.OSVersion, Architecture: input.Architecture,
		ToolID: "internal.executor", ActiveInstallationID: "builtin-self-test", ActiveVersion: "0.1.0", ActiveManager: "builtin",
		Installations: []InstallationSummary{{
			ID: "builtin-self-test", Version: "0.1.0", Path: "builtin://envmason/version", Manager: "builtin",
			ActiveState: "active", DefaultState: "default",
		}},
	}
	environmentDigest, err := digestValue(environment)
	if err != nil {
		return Plan{}, fmt.Errorf("build self-test plan: digest environment: %w", err)
	}
	policyDigest, err := digestValue(struct {
		Policy string `json:"policy"`
	}{Policy: "i14-self-test-v1"})
	if err != nil {
		return Plan{}, fmt.Errorf("build self-test plan: digest policy: %w", err)
	}
	createdAt := input.CreatedAt.UTC()
	value := Plan{
		SchemaVersion: ExecutableSchemaVersion,
		CreatedAt:     createdAt, ExpiresAt: createdAt.Add(DefaultTTL), Executable: true,
		Summary:           "Run the built-in read-only EnvMason version self-test and persist its local audit record.",
		EnvironmentDigest: environmentDigest, PolicyDigest: policyDigest, Environment: environment,
		Actions: []Action{{
			ID: "executor-self-test", ToolID: "internal.executor", Operation: "self_test", Adapter: "builtin", TargetVersion: "0.1.0",
			Risk: RiskR1, Dependencies: []string{}, Confirmation: Confirmation{Required: true, Scope: "plan"},
			ElevationRequired: false, RestartRequired: false, Download: Download{State: "known", Bytes: int64Pointer(0)},
			Preconditions: []Check{{Kind: "action_registered", Subject: "internal.executor/self_test/builtin", Expected: "true"}},
			Verifications: []Check{{Kind: "process_exit_zero", Subject: "executor-self-test", Expected: "true"}},
			Recovery:      Recovery{Mode: "manual", Summary: "The action changes no system state; its local audit record can be removed through a future confirmed history-management Plan."},
		}},
	}
	value.ID, err = planID(value)
	if err != nil {
		return Plan{}, fmt.Errorf("build self-test plan: calculate ID: %w", err)
	}
	if err := Validate(value); err != nil {
		return Plan{}, err
	}
	return value, nil
}

func int64Pointer(value int64) *int64 { return &value }
