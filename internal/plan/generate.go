package plan

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/report"
)

type Options struct {
	ToolID     string
	Format     Format
	Online     bool
	Projects   []string
	Excludes   []string
	PolicyPath string
}

func ValidateOptions(options Options) error {
	if options.ToolID != "runtime.node" {
		return fmt.Errorf("unsupported plan tool %q", options.ToolID)
	}
	if options.Format != "" && options.Format != FormatSummary && options.Format != FormatJSON {
		return fmt.Errorf("unsupported plan format %q", options.Format)
	}
	if !options.Online {
		return errors.New("plan preview requires --online for fresh version evidence")
	}
	if options.PolicyPath != "" && strings.TrimSpace(options.PolicyPath) == "" {
		return errors.New("policy path must not be empty")
	}
	return report.ValidateOptions(report.Options{Projects: options.Projects, Excludes: options.Excludes, PolicyPath: options.PolicyPath})
}

func Generate(ctx context.Context, options Options) ([]byte, error) {
	return generate(ctx, options, time.Now, report.Assess)
}

type assessFunc func(context.Context, report.Options) (report.AssessmentResult, error)

func generate(ctx context.Context, options Options, now func() time.Time, assess assessFunc) ([]byte, error) {
	if err := ValidateOptions(options); err != nil {
		return nil, err
	}
	result, err := assess(ctx, report.Options{Online: true, Projects: options.Projects, Excludes: options.Excludes, PolicyPath: options.PolicyPath})
	if err != nil {
		return nil, err
	}
	value, err := Build(BuildInput{Inventory: result.Inventory, Policy: result.Policy, Versions: result.Versions, CreatedAt: now(), TTL: DefaultTTL})
	if err != nil {
		return nil, err
	}
	return Render(value, options.Format)
}
