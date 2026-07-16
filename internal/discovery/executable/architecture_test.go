package executable

import (
	"debug/macho"
	"os"
	"runtime"
	"slices"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

func TestArchitectureForCPU(t *testing.T) {
	t.Parallel()

	for cpu, want := range map[macho.Cpu]inventory.Architecture{
		macho.Cpu386:   inventory.Architecture386,
		macho.CpuAmd64: inventory.ArchitectureAMD64,
		macho.CpuArm:   inventory.ArchitectureARM,
		macho.CpuArm64: inventory.ArchitectureARM64,
		macho.CpuPpc64: inventory.ArchitecturePPC64,
		macho.Cpu(999): inventory.ArchitectureUnknown,
	} {
		if got := architectureForCPU(cpu); got != want {
			t.Fatalf("architectureForCPU(%v) = %q, want %q", cpu, got, want)
		}
	}
}

func TestNormalizeArchitecturesIsStableAndUnique(t *testing.T) {
	t.Parallel()

	got := normalizeArchitectures([]inventory.Architecture{
		inventory.ArchitectureARM64,
		inventory.ArchitectureAMD64,
		inventory.ArchitectureARM64,
	})
	want := []inventory.Architecture{inventory.ArchitectureAMD64, inventory.ArchitectureARM64}
	if !slices.Equal(got, want) {
		t.Fatalf("architectures = %#v, want %#v", got, want)
	}
	if got := normalizeArchitectures(nil); !slices.Equal(got, []inventory.Architecture{inventory.ArchitectureUnknown}) {
		t.Fatalf("empty architectures = %#v", got)
	}
}

func TestMachoInspectorReadsCurrentTestBinary(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O integration requires macOS")
	}

	path, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	architectures, err := (machoInspector{}).Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	want := inventory.Architecture(runtime.GOARCH)
	if !slices.Contains(architectures, want) {
		t.Fatalf("architectures = %#v, want %q", architectures, want)
	}
}
