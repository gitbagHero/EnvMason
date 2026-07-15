package buildinfo

import "runtime"

// Values are replaced at build time with -ldflags. Safe defaults keep local
// development builds useful and make missing release metadata explicit.
var (
	Version   = "devel"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Info describes the EnvMason binary that is currently running.
type Info struct {
	Version   string
	Commit    string
	BuildTime string
	GoVersion string
	Target    string
}

// Current returns build metadata together with runtime toolchain and target
// information supplied by Go itself.
func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
		Target:    runtime.GOOS + "/" + runtime.GOARCH,
	}
}
