package backend

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	// Backend binary name (MLX server wrapper)
	binaryName = "mlx-server"
	
	// Default port for the backend server
	defaultPort = 11434
	
	// Model identifier
	modelID = "mlx-community/GLM-4.5-Air-3bit"
	
	// Download URLs for the bundled MLX server + model
	// This would be a custom package containing:
	// - Python runtime (embedded)
	// - MLX libraries
	// - Pre-downloaded GLM-4.5-Air-3bit model
	// - Launch script
	macOSARM64URL = "https://github.com/chasedut/toke-mlx-backend/releases/download/v0.1.0/mlx-glm-bundle-darwin-arm64.tar.gz"
	
	// Expected checksums for verification
	macOSARM64Checksum = "placeholder_checksum" // TODO: Update with actual checksum
	
	// Download timeout
	downloadTimeout = 60 * time.Minute // Longer timeout for large model
)

type Backend struct {
	binaryPath string
	dataDir    string
	port       int
	process    *os.Process
	cancelFunc context.CancelFunc
}

// New creates a new backend instance
func New(dataDir string) *Backend {
	return &Backend{
		dataDir: dataDir,
		port:    defaultPort,
	}
}

// IsInstalled checks if the backend binary is installed
func (b *Backend) IsInstalled() bool {
	binaryPath := b.getBinaryPath()
	info, err := os.Stat(binaryPath)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0111 != 0 // Check if executable
}

// IsRunning checks if the backend is currently running
func (b *Backend) IsRunning() bool {
	if b.process == nil {
		return false
	}
	
	// Check if process is still alive
	err := b.process.Signal(os.Signal(nil))
	return err == nil
}

// NeedsSetup returns true if the backend needs to be set up
func (b *Backend) NeedsSetup() bool {
	return !b.IsInstalled()
}

// GetDownloadSize returns the estimated download size in bytes
func (b *Backend) GetDownloadSize() int64 {
	// GLM-4.5-Air is actually a 107B parameter model
	// Even at 3-bit quantization, this is approximately:
	// - Model weights: ~40GB
	// - MLX runtime + Python: ~500MB
	// Total: ~40.5GB
	// This is probably too large for automatic download
	return 40 * 1024 * 1024 * 1024 // 40 GB - WARNING: Very large!
}

// GetPlatformSupport checks if the current platform is supported
func (b *Backend) GetPlatformSupport() (supported bool, reason string) {
	if runtime.GOOS != "darwin" {
		return false, "Currently only macOS is supported"
	}
	if runtime.GOARCH != "arm64" {
		return false, "Currently only Apple Silicon (M1/M2/M3) is supported"
	}
	return true, ""
}

// Download downloads the backend binary with progress callback
func (b *Backend) Download(ctx context.Context, progressFn func(downloaded, total int64)) error {
	supported, reason := b.GetPlatformSupport()
	if !supported {
		return fmt.Errorf("platform not supported: %s", reason)
	}
	
	// Get download URL and checksum for current platform
	downloadURL := macOSARM64URL
	expectedChecksum := macOSARM64Checksum
	
	// Create temp file for download
	tempFile, err := os.CreateTemp("", "toke-backend-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	
	// Download with timeout
	downloadCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	
	req, err := http.NewRequestWithContext(downloadCtx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}
	
	// Get total size for progress
	totalSize := resp.ContentLength
	
	// Create progress reader
	hasher := sha256.New()
	progressReader := &progressReader{
		reader:     resp.Body,
		total:      totalSize,
		progressFn: progressFn,
	}
	
	// Download and hash simultaneously
	writer := io.MultiWriter(tempFile, hasher)
	_, err = io.Copy(writer, progressReader)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	
	// Verify checksum
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if expectedChecksum != "placeholder_checksum" && actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}
	
	// Extract the archive
	if err := b.extractArchive(tempFile.Name()); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}
	
	slog.Info("Backend downloaded and installed successfully")
	return nil
}

// Start starts the backend server
func (b *Backend) Start(ctx context.Context) error {
	if !b.IsInstalled() {
		return fmt.Errorf("backend not installed")
	}
	
	if b.IsRunning() {
		slog.Info("Backend already running")
		return nil
	}
	
	binaryPath := b.getBinaryPath()
	
	// Create context for the backend process
	backendCtx, cancel := context.WithCancel(ctx)
	b.cancelFunc = cancel
	
	// Start the backend process
	cmd := exec.CommandContext(backendCtx, binaryPath, 
		"--port", fmt.Sprintf("%d", b.port),
		"--model-path", filepath.Join(b.dataDir, "models", "glm-4.5-air-3bit"),
	)
	
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start backend: %w", err)
	}
	
	b.process = cmd.Process
	
	// Wait for backend to be ready
	if err := b.waitForReady(ctx); err != nil {
		b.Stop()
		return fmt.Errorf("backend failed to start: %w", err)
	}
	
	slog.Info("Backend started successfully", "port", b.port)
	return nil
}

// Stop stops the backend server
func (b *Backend) Stop() error {
	if b.cancelFunc != nil {
		b.cancelFunc()
		b.cancelFunc = nil
	}
	
	if b.process != nil {
		// Give it time to shutdown gracefully
		done := make(chan error, 1)
		go func() {
			_, err := b.process.Wait()
			done <- err
		}()
		
		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill if not exited
			b.process.Kill()
		}
		
		b.process = nil
	}
	
	slog.Info("Backend stopped")
	return nil
}

// GetEndpoint returns the backend API endpoint
func (b *Backend) GetEndpoint() string {
	return fmt.Sprintf("http://localhost:%d/v1", b.port)
}

// Helper functions

func (b *Backend) getBinaryPath() string {
	return filepath.Join(b.dataDir, "bin", binaryName)
}

func (b *Backend) extractArchive(archivePath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()
	
	tr := tar.NewReader(gzr)
	
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		
		target := filepath.Join(b.dataDir, header.Name)
		
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			
			outFile.Close()
			
			// Set executable permissions if it's the binary
			if filepath.Base(target) == binaryName {
				if err := os.Chmod(target, 0755); err != nil {
					return err
				}
			}
		}
	}
	
	return nil
}

func (b *Backend) waitForReady(ctx context.Context) error {
	endpoint := b.GetEndpoint() + "/health"
	
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	timeout := time.After(30 * time.Second)
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for backend to be ready")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
			if err != nil {
				continue
			}
			
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

type progressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	progressFn func(downloaded, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)
	if pr.progressFn != nil {
		pr.progressFn(pr.downloaded, pr.total)
	}
	return n, err
}