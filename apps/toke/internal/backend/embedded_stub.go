//go:build !embedded

package backend

import (
	"fmt"
)

// Stub implementations when not building with embedded tag

func ExtractEmbeddedBackends(dataDir string) error {
	// No embedded backends in non-embedded build
	return fmt.Errorf("embedded backends not available in this build")
}

func HasEmbeddedBackends() bool {
	return false
}

func GetEmbeddedBackendPath(dataDir string, backend string) string {
	return ""
}

func GetEmbeddedNgrokPath(dataDir string) string {
	return ""
}