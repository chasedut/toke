package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Build-time parameters set via -ldflags
var (
	Version   = "v0.4201"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// Full version string
func Full() string {
	return fmt.Sprintf("%s (built %s, commit %s)", Version, BuildTime, GitCommit)
}

// Info returns detailed version information
func Info() string {
	return fmt.Sprintf(
		"Toke %s\nGo: %s\nOS/Arch: %s/%s\nBuilt: %s\nCommit: %s",
		Version,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		BuildTime,
		GitCommit,
	)
}

// A user may install toke using `go install github.com/chasedut/toke@latest`.
// without -ldflags, in which case the version above is unset. As a workaround
// we use the embedded build version that *is* set when using `go install` (and
// is only set for `go install` and not for `go build`).
func init() {
	// Only use build info if Version hasn't been set via ldflags
	// (checking if it's still the default value)
	if Version == "v0.4201" || Version == "unknown" {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			// < go v1.18
			return
		}
		mainVersion := info.Main.Version
		if mainVersion == "" || mainVersion == "(devel)" {
			// bin not built using `go install`
			return
		}
		// bin built using `go install`
		Version = mainVersion
	}
}
