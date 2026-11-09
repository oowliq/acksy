// Package buildinfo exposes version metadata injected at build time via -ldflags.
package buildinfo

// Set via -ldflags "-X github.com/oowliq/acksy/internal/buildinfo.Version=...".
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info is the serializable build metadata.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Get returns the current build metadata.
func Get() Info {
	return Info{Version: Version, Commit: Commit, Date: Date}
}
