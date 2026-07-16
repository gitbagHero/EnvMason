package executable

import (
	"debug/macho"
	"errors"
	"sort"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

type architectureInspector interface {
	Inspect(string) ([]inventory.Architecture, error)
}

type machoInspector struct{}

func (machoInspector) Inspect(path string) ([]inventory.Architecture, error) {
	fat, err := macho.OpenFat(path)
	if err == nil {
		defer fat.Close()
		architectures := make([]inventory.Architecture, 0, len(fat.Arches))
		for _, architecture := range fat.Arches {
			architectures = append(architectures, architectureForCPU(architecture.Cpu))
		}
		return normalizeArchitectures(architectures), nil
	}
	if !errors.Is(err, macho.ErrNotFat) {
		return nil, err
	}

	thin, err := macho.Open(path)
	if err != nil {
		return nil, err
	}
	defer thin.Close()
	return []inventory.Architecture{architectureForCPU(thin.Cpu)}, nil
}

func architectureForCPU(cpu macho.Cpu) inventory.Architecture {
	switch cpu {
	case macho.Cpu386:
		return inventory.Architecture386
	case macho.CpuAmd64:
		return inventory.ArchitectureAMD64
	case macho.CpuArm:
		return inventory.ArchitectureARM
	case macho.CpuArm64:
		return inventory.ArchitectureARM64
	case macho.CpuPpc64:
		return inventory.ArchitecturePPC64
	default:
		return inventory.ArchitectureUnknown
	}
}

func normalizeArchitectures(values []inventory.Architecture) []inventory.Architecture {
	seen := make(map[inventory.Architecture]struct{}, len(values))
	result := make([]inventory.Architecture, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	if len(result) == 0 {
		return []inventory.Architecture{inventory.ArchitectureUnknown}
	}
	return result
}
