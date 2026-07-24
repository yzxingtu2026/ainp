// Package buildinfo exposes the immutable identity of a compiled AINP artifact.
package buildinfo

// These values are overridden by release builds through Go linker flags.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// Info is the public, non-sensitive identity shared by health checks and logs.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// Current returns the identity embedded in this binary.
func Current() Info {
	return Info{Version: version, Commit: commit, BuildTime: buildTime}
}
