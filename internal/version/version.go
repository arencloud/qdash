package version

import "fmt"

var (
	// These values are set at build time via -ldflags.
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func String() string {
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, BuildDate)
}
