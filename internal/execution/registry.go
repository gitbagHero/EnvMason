package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

type ActionKey struct {
	ToolID    string
	Operation string
	Adapter   string
}

func (key ActionKey) String() string { return key.ToolID + "/" + key.Operation + "/" + key.Adapter }

type Definition struct {
	Key         ActionKey
	MinimumRisk plan.Risk
	Build       func(plan.Action) (CommandSpec, error)
	Preflight   func(context.Context, plan.Action) error
	Verify      func(context.Context, plan.Action, ProcessResult) error
}

type Registry struct {
	definitions map[string]Definition
}

func NewRegistry(definitions ...Definition) (Registry, error) {
	registry := Registry{definitions: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		key := definition.Key.String()
		if definition.Key.ToolID == "" || definition.Key.Operation == "" || definition.Key.Adapter == "" ||
			definition.Build == nil || definition.Verify == nil || riskRank(definition.MinimumRisk) < riskRank(plan.RiskR1) {
			return Registry{}, errors.New("execution registry: invalid definition")
		}
		if _, duplicate := registry.definitions[key]; duplicate {
			return Registry{}, fmt.Errorf("execution registry: duplicate action %q", key)
		}
		registry.definitions[key] = definition
	}
	return registry, nil
}

func (registry Registry) Resolve(action plan.Action) (Definition, error) {
	key := (ActionKey{ToolID: action.ToolID, Operation: action.Operation, Adapter: action.Adapter}).String()
	definition, ok := registry.definitions[key]
	if !ok {
		return Definition{}, executionError(CodeActionUnregistered, "plan action is not registered by the deterministic core")
	}
	if riskRank(action.Risk) < riskRank(definition.MinimumRisk) {
		return Definition{}, executionError(CodePlanInvalid, "plan action risk is lower than the registered minimum")
	}
	return definition, nil
}

func SelfTestDefinition(executable string) Definition {
	return Definition{
		Key:         ActionKey{ToolID: "internal.executor", Operation: "self_test", Adapter: "builtin"},
		MinimumRisk: plan.RiskR1,
		Build: func(plan.Action) (CommandSpec, error) {
			return CommandSpec{Executable: executable, Args: []string{"version"}, Environment: []string{}, Timeout: 5 * time.Second}, nil
		},
		Verify: func(_ context.Context, _ plan.Action, result ProcessResult) error {
			if result.ExitCode == nil || *result.ExitCode != 0 || !strings.HasPrefix(result.Stdout.Text, "envmason ") {
				return errors.New("version self-test output did not match")
			}
			return nil
		},
	}
}

func riskRank(value plan.Risk) int {
	switch value {
	case plan.RiskR0:
		return 0
	case plan.RiskR1:
		return 1
	case plan.RiskR2:
		return 2
	case plan.RiskR3:
		return 3
	case plan.RiskR4:
		return 4
	default:
		return -1
	}
}
