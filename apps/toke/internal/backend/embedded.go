//go:build embedded

package backend

import (
	"archive/tar"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Embed the compressed backend binaries directly into the Go binary
// These files need to exist at build time
//
//go:embed all:embedded_backends
var embeddedBackends embed.FS

// ExtractEmbeddedBackends extracts and decompresses the embedded backend binaries to the data directory
func ExtractEmbeddedBackends(dataDir string) error {
	// Only extract for darwin/arm64 for now
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return fmt.Errorf("embedded backends only available for darwin/arm64")
	}

	backendsDir := filepath.Join(dataDir, "backends")
	if err := os.MkdirAll(backendsDir, 0755); err != nil {
		return fmt.Errorf("failed to create backends directory: %w", err)
	}

	// Check if already extracted by looking for marker file with version
	markerFile := filepath.Join(backendsDir, ".extracted-v3") // v3 includes ngrok
	if _, err := os.Stat(markerFile); err == nil {
		// Check if all expected files exist
		expectedFiles := []string{"llama-server", "mlx-server", "mlx_server.py", "mlx-env", "ngrok"}
		allExist := true
		for _, file := range expectedFiles {
			path := filepath.Join(backendsDir, file)
			if _, err := os.Stat(path); err != nil {
				allExist = false
				break
			}
		}
		if allExist {
			slog.Info("Embedded backends already extracted", "dir", backendsDir)
			return nil
		}
	}

	slog.Info("Extracting compressed embedded backends", "dir", backendsDir)

	// Walk through embedded files and extract them
	err := fs.WalkDir(embeddedBackends, "embedded_backends", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory
		if path == "embedded_backends" {
			return nil
		}

		// Skip directories - we only process compressed files
		if d.IsDir() {
			return nil
		}

		// Calculate destination path
		relPath, err := filepath.Rel("embedded_backends", path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Handle different file types
		if strings.HasSuffix(relPath, ".tar.gz") {
			// Extract tar.gz archives (like mlx-env.tar.gz)
			return extractTarGz(path, backendsDir)
		} else if strings.HasSuffix(relPath, ".gz") {
			// Extract gzipped files
			baseName := strings.TrimSuffix(relPath, ".gz")
			destPath := filepath.Join(backendsDir, baseName)
			return extractGzFile(path, destPath)
		} else {
			// Copy uncompressed files as-is
			destPath := filepath.Join(backendsDir, relPath)
			return copyEmbeddedFile(path, destPath, d)
		}
	})

	if err != nil {
		return fmt.Errorf("failed to extract embedded backends: %w", err)
	}

	// Create marker file to indicate successful extraction
	if err := os.WriteFile(markerFile, []byte("extracted-v3"), 0644); err != nil {
		slog.Warn("Failed to create extraction marker file", "error", err)
	}

	slog.Info("Successfully extracted compressed backends", "dir", backendsDir)
	return nil
}

func extractGzFile(embeddedPath, destPath string) error {
	// Open embedded gzipped file
	srcFile, err := embeddedBackends.Open(embeddedPath)
	if err != nil {
		return fmt.Errorf("failed to open embedded file %s: %w", embeddedPath, err)
	}
	defer srcFile.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(srcFile)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader for %s: %w", embeddedPath, err)
	}
	defer gzReader.Close()

	// Create destination file
	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", destPath, err)
	}
	defer destFile.Close()

	// Copy decompressed content
	if _, err := io.Copy(destFile, gzReader); err != nil {
		return fmt.Errorf("failed to decompress %s: %w", embeddedPath, err)
	}

	// Make executable if it's a binary
	baseName := filepath.Base(destPath)
	if baseName == "llama-server" || baseName == "mlx-server" || baseName == "mlx_server.py" || baseName == "ngrok" {
		if err := os.Chmod(destPath, 0755); err != nil {
			return fmt.Errorf("failed to make %s executable: %w", destPath, err)
		}
	}

	slog.Debug("Extracted compressed file", "from", embeddedPath, "to", destPath)
	return nil
}

func extractTarGz(embeddedPath, destDir string) error {
	// Open embedded tar.gz file
	srcFile, err := embeddedBackends.Open(embeddedPath)
	if err != nil {
		return fmt.Errorf("failed to open embedded file %s: %w", embeddedPath, err)
	}
	defer srcFile.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(srcFile)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader for %s: %w", embeddedPath, err)
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	// Extract files from tar
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Calculate destination path
		destPath := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
		case tar.TypeReg:
			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// Create file
			file, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", destPath, err)
			}

			// Copy file contents
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return fmt.Errorf("failed to extract file %s: %w", destPath, err)
			}
			file.Close()
		}
	}

	slog.Debug("Extracted tar.gz archive", "from", embeddedPath, "to", destDir)
	return nil
}

func copyEmbeddedFile(embeddedPath, destPath string, d fs.DirEntry) error {
	// Open embedded file
	srcFile, err := embeddedBackends.Open(embeddedPath)
	if err != nil {
		return fmt.Errorf("failed to open embedded file %s: %w", embeddedPath, err)
	}
	defer srcFile.Close()

	// Get file info for permissions
	info, err := d.Info()
	if err != nil {
		return fmt.Errorf("failed to get file info for %s: %w", embeddedPath, err)
	}

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Create destination file
	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", destPath, err)
	}
	defer destFile.Close()

	// Copy file contents
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file %s: %w", embeddedPath, err)
	}

	slog.Debug("Copied embedded file", "from", embeddedPath, "to", destPath)
	return nil
}

// HasEmbeddedBackends checks if embedded backends are available
func HasEmbeddedBackends() bool {
	// Try to check if the embedded directory exists
	if _, err := embeddedBackends.ReadDir("embedded_backends"); err != nil {
		return false
	}
	return true
}

// GetEmbeddedBackendPath returns the path where embedded backends should be extracted
func GetEmbeddedBackendPath(dataDir string, backend string) string {
	return filepath.Join(dataDir, "backends", backend)
}

// GetEmbeddedNgrokPath returns the path to the embedded ngrok binary
func GetEmbeddedNgrokPath(dataDir string) string {
	return filepath.Join(dataDir, "backends", "ngrok")
}