package version

import (
	"fmt"
	"runtime/debug"
)

var (
	// Version is the main version number that is being run at the moment.
	Version = "dev"
	// Commit is the git commit that was compiled.
	Commit = "unknown"
	// BuildDate is the time of compilation.
	BuildDate = "unknown"
	// GoVersion is the version of Go used to build the binary.
	GoVersion = "unknown"
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		GoVersion = info.GoVersion
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				Commit = setting.Value
			case "vcs.time":
				BuildDate = setting.Value
			}
		}
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}
}

// Info returns a string containing build information.
func Info() string {
	return fmt.Sprintf("rss2go version %s, commit %s, built at %s with %s", Version, Commit, BuildDate, GoVersion)
}
