package buildinfo

// These values are replaced by -ldflags during release builds.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)
