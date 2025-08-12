package backend

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
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
	
	// Create bin directory
	if err := os.MkdirAll(filepath.Dir(serverPath), 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}
	
	// Download bundled MLX server (includes Python, mlx-lm, and dependencies)
	downloadURL := "https://github.com/chasedut/toke-mlx-server/releases/latest/download/mlx-server-darwin-arm64.tar.gz"
	
	// Allow override for testing
	if testURL := os.Getenv("TOKE_MLX_SERVER_URL"); testURL != "" {
		downloadURL = testURL
		slog.Info("Using custom mlx-server URL", "url", downloadURL)
	}
	
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
}

// DownloadModel downloads the MLX model from Hugging Face
func (b *MLXBackend) DownloadModel(ctx context.Context, model ModelOption, progressFn func(downloaded, total int64)) error {
	modelPath := filepath.Join(b.dataDir, "models", "mlx", model.ID)
	
	// Check if model directory exists and has required files
	requiredFiles := []string{"config.json", "model.safetensors"}
	allFilesExist := true
	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(modelPath, file)); err != nil {
			allFilesExist = false
			break
		}
	}
	
	if allFilesExist {
		b.modelPath = modelPath
		slog.Info("Model already downloaded", "path", modelPath)
		if progressFn != nil {
			progressFn(model.Size, model.Size)
		}
		return nil
	}
	
	// Create model directory
	if err := os.MkdirAll(modelPath, 0755); err != nil {
		return fmt.Errorf("failed to create model directory: %w", err)
	}
	
	// For MLX models, we need to download multiple files
	// This is simplified - in production, we'd use git or huggingface-hub
	files := []struct {
		name string
		url  string
	}{
		{"config.json", fmt.Sprintf("%s/resolve/main/config.json", model.URL)},
		{"model.safetensors", fmt.Sprintf("%s/resolve/main/model.safetensors", model.URL)},
		{"tokenizer.json", fmt.Sprintf("%s/resolve/main/tokenizer.json", model.URL)},
		{"tokenizer_config.json", fmt.Sprintf("%s/resolve/main/tokenizer_config.json", model.URL)},
	}
	
	totalDownloaded := int64(0)
	
	for _, file := range files {
		filePath := filepath.Join(modelPath, file.name)
		
		// Skip if file exists
		if _, err := os.Stat(filePath); err == nil {
			continue
		}
		
		slog.Info("Downloading model file", "file", file.name, "url", file.url)
		
		req, err := http.NewRequestWithContext(ctx, "GET", file.url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request for %s: %w", file.name, err)
		}
		
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
		
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", file.name, err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download %s: %s", file.name, resp.Status)
		}
		
		// Create file
		out, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", file.name, err)
		}
		
		// Copy with progress tracking for large files
		if file.name == "model.safetensors" && progressFn != nil {
			progressReader := &progressReader{
				reader:     resp.Body,
				total:      model.Size,
				downloaded: totalDownloaded,
				progressFn: progressFn,
			}
			_, err = io.Copy(out, progressReader)
			totalDownloaded = progressReader.downloaded
		} else {
			_, err = io.Copy(out, resp.Body)
		}
		
		out.Close()
		if err != nil {
			return fmt.Errorf("failed to write %s: %w", file.name, err)
		}
	}
	
	b.modelPath = modelPath
	slog.Info("Model downloaded successfully", "path", modelPath)
	return nil
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
	
	// The mlx-server script will handle launching Python with the bundled environment
	args := []string{
		"--model", b.modelPath,
		"--port", fmt.Sprintf("%d", b.port),
		"--host", "127.0.0.1",
		"--max-tokens", "4096",
		"--trust-remote-code", // Required for some models
	}
	
	// Start the server
	b.process = exec.CommandContext(ctx, b.binaryPath, args...)
	
	// Set environment to use bundled Python
	envPath := filepath.Join(b.dataDir, "bin", "mlx-env")
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
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", b.port))
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