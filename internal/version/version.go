package version

import (
	"fmt"
	"runtime"
)

// Version is the current version of ioetap.
// This is set to the development version by default and overridden
// at build time using ldflags for release builds:
//
//	go build -ldflags "-X github.com/trustin/ioetap/internal/version.Version=1.0.0"
var Version = "1.0.0-dev"

// GitCommit is the git commit hash of the build.
// This is set at build time using ldflags:
//
//	go build -ldflags "-X github.com/trustin/ioetap/internal/version.GitCommit=$(git rev-parse --short HEAD)"
var GitCommit = ""

// BuildTime is the time the binary was built.
// This is set at build time using ldflags:
//
//	go build -ldflags "-X github.com/trustin/ioetap/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var BuildTime = ""

// Info returns the full version information string.
func Info() string {
	info := fmt.Sprintf("ioetap %s", Version)

	if GitCommit != "" {
		info += fmt.Sprintf(" (%s)", GitCommit)
	}

	if BuildTime != "" {
		info += fmt.Sprintf(" built %s", BuildTime)
	}

	info += fmt.Sprintf(" %s/%s", runtime.GOOS, runtime.GOARCH)

	return info
}
