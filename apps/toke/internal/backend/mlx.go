package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Ensure MLXBackend implements ModelBackend interface
var _ ModelBackend = (*MLXBackend)(nil)

// MLXBackend manages an MLX server instance
type MLXBackend struct {
	binaryPath string
	modelPath  string
	dataDir    string
	port       int
	socketPath string
	process    *exec.Cmd
	modelID    string
}

// NewMLXBackend creates a new MLX backend
func NewMLXBackend(dataDir string, modelID string) *MLXBackend {
	return &MLXBackend{
		dataDir: dataDir,
		port:    11435, // Different port from llama.cpp
		modelID: modelID,
	}
}

// DownloadServer downloads the MLX server binary bundle
func (b *MLXBackend) DownloadServer(ctx context.Context, progressFn func(downloaded, total int64)) error {
	// First try to extract embedded backends if available
	if HasEmbeddedBackends() {
		if err := ExtractEmbeddedBackends(b.dataDir); err != nil {
			slog.Warn("Failed to extract embedded backends", "error", err)
		} else {
			// Check for extracted MLX server
			embeddedPath := GetEmbeddedBackendPath(b.dataDir, "mlx-server")
			if _, err := os.Stat(embeddedPath); err == nil {
				b.binaryPath = embeddedPath
				slog.Info("Using embedded MLX server", "path", embeddedPath)
				return nil
			}
		}
	}
	
	// Check for bundled MLX server in executable directory
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		bundledPath := filepath.Join(execDir, "backends", "mlx-server")
		if _, err := os.Stat(bundledPath); err == nil {
			b.binaryPath = bundledPath
			slog.Info("Using bundled MLX server", "path", bundledPath)
			return nil
		}
	}
	
	// Fall back to checking in data directory
	serverPath := filepath.Join(b.dataDir, "bin", "mlx-server")
	
	// Check if already exists
	if _, err := os.Stat(serverPath); err == nil {
		b.binaryPath = serverPath
		return nil
	}
	
	// Only support Apple Silicon for now
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return fmt.Errorf("MLX is only supported on Apple Silicon Macs")
	}
	
	// No bundled version found and no downloaded version
	return fmt.Errorf("MLX server not found. Please ensure the bundled version is included in the application")
	
	// TODO: Implement proper download when MLX server bundles are available
	// The code below is for future use when we have pre-built bundles
	/*
	downloadURL := "https://github.com/chasedut/toke-mlx-server/releases/latest/download/mlx-server-darwin-arm64.tar.gz"
	slog.Info("Downloading MLX server bundle", "url", downloadURL)
	
	// Download the server bundle
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("User-Agent", "Toke/1.0")
	
	client := &http.Client{
		Timeout: 10 * time.Minute, // Larger bundle than llama.cpp
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download server: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download server: %s (status: %d)", resp.Status, resp.StatusCode)
	}
	
	// Extract tar.gz to bin directory
	tempDir, err := os.MkdirTemp(filepath.Dir(serverPath), "mlx-server-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Download with progress
	progressReader := &progressReader{
		reader:     resp.Body,
		total:      resp.ContentLength,
		progressFn: progressFn,
	}
	
	// Decompress and extract
	gzReader, err := gzip.NewReader(progressReader)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()
	
	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}
		
		targetPath := filepath.Join(tempDir, header.Name)
		
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			// Create directory if needed
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}
			
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return fmt.Errorf("failed to extract file: %w", err)
			}
			file.Close()
		}
	}
	
	// Move the mlx-server script to final location
	extractedServerPath := filepath.Join(tempDir, "mlx-server")
	if err := os.Rename(extractedServerPath, serverPath); err != nil {
		return fmt.Errorf("failed to move server to final location: %w", err)
	}
	
	// Move the Python environment
	extractedEnvPath := filepath.Join(tempDir, "mlx-env")
	envPath := filepath.Join(b.dataDir, "bin", "mlx-env")
	if err := os.Rename(extractedEnvPath, envPath); err != nil {
		return fmt.Errorf("failed to move Python environment: %w", err)
	}
	
	b.binaryPath = serverPath
	slog.Info("MLX server downloaded successfully", "path", serverPath)
	return nil
	*/
}

// DownloadModel downloads the MLX model from Hugging Face
func (b *MLXBackend) DownloadModel(ctx context.Context, model ModelOption, progressFn func(downloaded, total int64)) error {
	// Use the new download function that handles multi-part files
	return b.DownloadMLXModel(ctx, model, progressFn)
}

// Start starts the MLX server
func (b *MLXBackend) Start(ctx context.Context) error {
	if b.binaryPath == "" || b.modelPath == "" {
		return fmt.Errorf("server or model not downloaded")
	}
	
	// Check if already running
	if b.process != nil && b.process.ProcessState == nil {
		return nil
	}
	
	// Use Unix socket for better performance
	b.socketPath = filepath.Join(b.dataDir, "mlx-server.sock")
	
	// Remove existing socket if present
	os.Remove(b.socketPath)
	
	// The mlx-server script will handle launching Python with the bundled environment
	args := []string{
		"--model", b.modelPath,
		"--socket", b.socketPath,
		"--max-tokens", "4096",
		"--max-models", "3", // Cache up to 3 models
		"--trust-remote-code", // Required for some models
	}
	
	// Start the server
	b.process = exec.CommandContext(ctx, b.binaryPath, args...)
	
	// Set environment to use bundled Python
	// First check for embedded/extracted environment
	envPath := GetEmbeddedBackendPath(b.dataDir, "mlx-env")
	if _, err := os.Stat(envPath); err != nil {
		// Fall back to checking in executable directory
		if execPath, err := os.Executable(); err == nil {
			execDir := filepath.Dir(execPath)
			bundledEnvPath := filepath.Join(execDir, "backends", "mlx-env")
			if _, err := os.Stat(bundledEnvPath); err == nil {
				envPath = bundledEnvPath
				slog.Info("Using bundled MLX environment", "path", envPath)
			}
		}
		// Final fallback to bin directory
		if _, err := os.Stat(envPath); err != nil {
			envPath = filepath.Join(b.dataDir, "bin", "mlx-env")
		}
	} else {
		slog.Info("Using embedded MLX environment", "path", envPath)
	}
	
	b.process.Env = append(os.Environ(),
		fmt.Sprintf("MLX_ENV_PATH=%s", envPath),
		"PYTORCH_ENABLE_MPS_FALLBACK=1", // Fallback for unsupported ops
	)
	
	// Capture output for debugging
	stdout, err := b.process.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	
	stderr, err := b.process.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	
	// Start the process
	if err := b.process.Start(); err != nil {
		return fmt.Errorf("failed to start MLX server: %w", err)
	}
	
	// Log output in background
	go b.logOutput(stdout, "stdout")
	go b.logOutput(stderr, "stderr")
	
	// Wait for server to be ready
	if err := b.waitForReady(ctx); err != nil {
		b.Stop()
		return fmt.Errorf("server failed to start: %w", err)
	}
	
	slog.Info("MLX server started successfully", "port", b.port, "model", b.modelID)
	return nil
}

// Stop stops the MLX server
func (b *MLXBackend) Stop() error {
	if b.process == nil {
		return nil
	}
	
	// Try graceful shutdown first
	if err := b.process.Process.Signal(os.Interrupt); err != nil {
		// Force kill if interrupt fails
		return b.process.Process.Kill()
	}
	
	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- b.process.Wait()
	}()
	
	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(5 * time.Second):
		// Force kill after timeout
		b.process.Process.Kill()
		<-done
	}
	
	b.process = nil
	slog.Info("MLX server stopped")
	return nil
}

// GetEndpoint returns the OpenAI-compatible API endpoint
func (b *MLXBackend) GetEndpoint() string {
	// If using Unix socket, return a special format
	// The HTTP client will need to handle this specially
	if b.socketPath != "" {
		return fmt.Sprintf("unix://%s:/v1", b.socketPath)
	}
	return fmt.Sprintf("http://localhost:%d/v1", b.port)
}

// IsRunning checks if the server is running
func (b *MLXBackend) IsRunning() bool {
	if b.process == nil {
		return false
	}
	
	// Check if process has exited
	if b.process.ProcessState != nil {
		return false
	}
	
	// Check health endpoint
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://localhost:%d/health", b.port), nil)
	if err != nil {
		return false
	}
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	
	return resp.StatusCode == http.StatusOK
}

// Helper functions

func (b *MLXBackend) waitForReady(ctx context.Context) error {
	healthURL := fmt.Sprintf("http://localhost:%d/v1/models", b.port) // MLX uses /v1/models for health
	
	ticker := time.NewTicker(1 * time.Second) // MLX takes longer to start
	defer ticker.Stop()
	
	timeout := time.After(60 * time.Second) // Longer timeout for model loading
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for server to be ready")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
			if err != nil {
				continue
			}
			
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}
			
			if resp.StatusCode == http.StatusOK {
				// Check if model is actually loaded
				var models struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				}
				
				if err := json.NewDecoder(resp.Body).Decode(&models); err == nil {
					resp.Body.Close()
					if len(models.Data) > 0 {
						return nil
					}
				}
			}
			resp.Body.Close()
		}
	}
}

func (b *MLXBackend) logOutput(pipe io.Reader, name string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "error") || strings.Contains(line, "ERROR") {
			slog.Error("MLX server output", "stream", name, "line", line)
		} else if strings.Contains(line, "Model loaded") || strings.Contains(line, "Server running") {
			slog.Info("MLX server output", "stream", name, "line", line)
		} else {
			slog.Debug("MLX server output", "stream", name, "line", line)
		}
	}
}