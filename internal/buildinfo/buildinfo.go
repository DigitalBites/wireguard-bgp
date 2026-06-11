package buildinfo

import (
	"os"
	"strings"
	"time"
)

const EnvVar = "APP_BUILD_VERSION"

func FromEnv() string {
	return Resolve(os.Getenv(EnvVar), time.Now().UTC())
}

func Resolve(value string, now time.Time) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return "local-" + now.UTC().Format("2006-01-02T15:04:05Z")
}
