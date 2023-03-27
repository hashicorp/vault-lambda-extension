package config

import (
	"fmt"
	"runtime"
)

// GetUserAgentBase returns a base user agent string with the given user agent name and version in the form:
// vault-client-go/0.0.1 (Darwin arm64; Go go1.19.2)
func GetUserAgentBase(clientName string, clientVersion string) string {
	return fmt.Sprintf("%s/%s (%s %s; Go %s)", clientName, clientVersion, runtime.GOOS, runtime.GOARCH, runtime.Version())
}
