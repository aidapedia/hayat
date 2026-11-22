// Package version is the release this binary was built from. It is a leaf so
// every role can report it: the console's overview, `hayat version`, and the
// api role's ServerInfoService, which services read to refuse a Hayat older
// than they need.
package version

// Version is set at build time with -ldflags
// "-X github.com/aidapedia/hayat/internal/version.Version=1.2.0" — the release
// without its leading v. A build from source says "dev".
var Version = "dev"
