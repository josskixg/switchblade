// Package version holds build-time version information injected via -ldflags.
package version

// These variables are set at build time:
//
//	go build -ldflags="-X switchblade/internal/version.Version=1.0.0 ..."
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
	GoVer   = "unknown"
)

// Info returns a map with all version fields.
func Info() map[string]string {
	return map[string]string{
		"version": Version,
		"commit":  Commit,
		"date":    Date,
		"go":      GoVer,
	}
}
