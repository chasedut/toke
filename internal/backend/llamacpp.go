package backend

import (
	"bufio"
	"compress/gzip"
	"context"
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

// Ensure LlamaCppBackend implements ModelBackend interface
var _ ModelBackend = (*LlamaCppBackend)(nil)

// LlamaCppBackend manages a llama.cpp server instance
type LlamaCppBackend struct {
	binaryPath string
	modelPath  string
	dataDir    string
	port       int
	process    *exec.Cmd
	modelID    string
}

// NewLlamaCppBackend creates a new llama.cpp backend
func NewLlamaCppBackend(dataDir string, modelID string) *LlamaCppBackend {
	return &LlamaCppBackend{
		dataDir: dataDir,
		port:    11434,
		modelID: modelID,
	}
}

// DownloadServer downloads the llama.cpp server binary
func (b *LlamaCppBackend) DownloadServer(ctx context.Context, progressFn func(downloaded, total int64)) error {
	serverPath := filepath.Join(b.dataDir, "bin", "llama-server")
	
	// Check if already exists
	if _, err := os.Stat(serverPath); err == nil {
		b.binaryPath = serverPath
		return nil
	}
	
	// Create bin directory
	if err := os.MkdirAll(filepath.Dir(serverPath), 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}
	
	// Download URL based on platform
	var downloadURL string
	baseURL := "https://github.com/chasedut/toke-llama-server/releases/latest/download/"
	
	// Allow override for testing
	if testURL := os.Getenv("TOKE_LLAMA_SERVER_URL"); testURL != "" {
		baseURL = testURL
		slog.Info("Using custom llama-server URL", "url", baseURL)
	}
	
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			// M1/M2/M3 Macs
			downloadURL = baseURL + "llama-server-darwin-arm64.gz"
		} else {
			downloadURL = baseURL + "llama-server-darwin-x64.gz"
		}
	case "linux":
		downloadURL = baseURL + "llama-server-linux-x64.gz"
	case "windows":
		downloadURL = baseURL + "llama-server-windows-x64.exe.zip"
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	
	slog.Info("Downloading llama-server", "url", downloadURL)
	
	// Download the compressed server
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("User-Agent", "Toke/1.0")
	
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download server: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		// Fallback: check if llama-server is in PATH
		if path, err := exec.LookPath("llama-server"); err == nil {
			slog.Info("Using system llama-server", "path", path)
			b.binaryPath = path
			return nil
		}
		return fmt.Errorf("failed to download server: %s (status: %d). Please install llama.cpp manually: brew install llama.cpp", resp.Status, resp.StatusCode)
	}
	
	// Create temp file for download
	tempFile, err := os.CreateTemp(filepath.Dir(serverPath), "llama-server-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	
	// Download with progress
	progressReader := &progressReader{
		reader:     resp.Body,
		total:      resp.ContentLength,
		progressFn: progressFn,
	}
	
	// Decompress based on format
	var reader io.Reader = progressReader
	if strings.HasSuffix(downloadURL, ".gz") {
		gzReader, err := gzip.NewReader(progressReader)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}
	
	_, err = io.Copy(tempFile, reader)
	if err != nil {
		return fmt.Errorf("failed to download server: %w", err)
	}
	
	// Make executable
	if err := tempFile.Chmod(0755); err != nil {
		return fmt.Errorf("failed to make server executable: %w", err)
	}
	
	tempFile.Close()
	
	// Move to final location
	if err := os.Rename(tempFile.Name(), serverPath); err != nil {
		return fmt.Errorf("failed to move server to final location: %w", err)
	}
	
	b.binaryPath = serverPath
	slog.Info("llama-server downloaded successfully", "path", serverPath)
	return nil
}

// DownloadModel downloads the GGUF model
func (b *LlamaCppBackend) DownloadModel(ctx context.Context, model ModelOption, progressFn func(downloaded, total int64)) error {
	modelPath := filepath.Join(b.dataDir, "models", model.ID+".gguf")
	
	// Check if already exists
	if info, err := os.Stat(modelPath); err == nil {
		if info.Size() == model.Size {
			b.modelPath = modelPath
			slog.Info("Model already downloaded", "path", modelPath)
			// Report 100% progress
			if progressFn != nil {
				progressFn(model.Size, model.Size)
			}
			return nil
		}
		// Size mismatch, remove corrupted file
		slog.Warn("Removing corrupted model file", "path", modelPath, "expected", model.Size, "actual", info.Size())
		os.Remove(modelPath)
	}
	
	// Create models directory
	if err := os.MkdirAll(filepath.Dir(modelPath), 0755); err != nil {
		return fmt.Errorf("failed to create models directory: %w", err)
	}
	
	// Check for stuck partial download (older than 5 minutes with no progress)
	partialPath := modelPath + ".partial"
	var startByte int64
	if info, err := os.Stat(partialPath); err == nil {
		// Check if file is stale (not modified in last 5 minutes)
		if time.Since(info.ModTime()) > 5*time.Minute {
			slog.Info("Removing stale partial download", "path", partialPath, "age", time.Since(info.ModTime()))
			os.Remove(partialPath)
			startByte = 0
		} else {
			startByte = info.Size()
			slog.Info("Resuming download", "path", partialPath, "startByte", startByte)
		}
	}
	
	// Download the model
	slog.Info("Downloading model", "url", model.URL, "startByte", startByte)
	req, err := http.NewRequestWithContext(ctx, "GET", model.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	// Add user agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	
	// Support resume if we have partial data
	if startByte > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
		slog.Info("Resuming download", "range", req.Header.Get("Range"))
		// Report initial progress
		if progressFn != nil {
			progressFn(startByte, model.Size)
		}
	}
	
	// Create custom client that follows redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Copy the Range header to the redirect request if present
			if rangeHeader := via[0].Header.Get("Range"); rangeHeader != "" {
				req.Header.Set("Range", rangeHeader)
			}
			return nil
		},
		Timeout: 0, // No timeout for large downloads
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download model: %w", err)
	}
	defer resp.Body.Close()
	
	slog.Info("Download response", "status", resp.StatusCode, "statusText", resp.Status, "contentLength", resp.ContentLength)
	
	// Accept 200 (OK), 206 (Partial Content), and check for failed resume
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && startByte > 0 {
		// Server doesn't support resume, start from beginning
		slog.Info("Server doesn't support resume, starting from beginning")
		os.Remove(partialPath)
		startByte = 0
		
		// Retry without Range header
		req.Header.Del("Range")
		resp.Body.Close()
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to download model (retry): %w", err)
		}
		defer resp.Body.Close()
	}
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("download failed with status: %s (code: %d)", resp.Status, resp.StatusCode)
	}
	
	// Open file for writing (append if resuming)
	var file *os.File
	if startByte > 0 {
		// Try to open for append, but create if it doesn't exist
		file, err = os.OpenFile(partialPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			// If that fails, just create a new file
			slog.Warn("Failed to open partial file for append, creating new", "error", err)
			startByte = 0
			file, err = os.Create(partialPath)
		}
	} else {
		file, err = os.Create(partialPath)
	}
	if err != nil {
		return fmt.Errorf("failed to create model file: %w", err)
	}
	
	// Download with progress
	progressReader := &progressReader{
		reader:     resp.Body,
		total:      model.Size,
		downloaded: startByte,
		progressFn: progressFn,
	}
	
	_, err = io.Copy(file, progressReader)
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to download model: %w", err)
	}
	
	// Close file before renaming (important for file locks)
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close model file: %w", err)
	}
	
	// Ensure target directory exists
	if err := os.MkdirAll(filepath.Dir(modelPath), 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}
	
	// Try to rename first (fastest option)
	renameErr := os.Rename(partialPath, modelPath)
	if renameErr != nil {
		// If rename fails (e.g., cross-device), fall back to copy and delete
		slog.Warn("Rename failed, trying copy instead", "error", renameErr)
		
		// Copy the file
		srcFile, err := os.Open(partialPath)
		if err != nil {
			return fmt.Errorf("failed to open partial file for copy: %w", err)
		}
		defer srcFile.Close()
		
		dstFile, err := os.Create(modelPath)
		if err != nil {
			return fmt.Errorf("failed to create final model file: %w", err)
		}
		
		_, copyErr := io.Copy(dstFile, srcFile)
		dstFile.Close()
		
		if copyErr != nil {
			os.Remove(modelPath) // Clean up partial copy
			return fmt.Errorf("failed to copy model file: %w", copyErr)
		}
		
		// Remove the partial file
		if err := os.Remove(partialPath); err != nil {
			slog.Warn("Failed to remove partial file after copy", "path", partialPath, "error", err)
		}
	}
	
	b.modelPath = modelPath
	slog.Info("Model downloaded successfully", "path", modelPath, "size", model.Size)
	return nil
}

// Start starts the llama.cpp server
func (b *LlamaCppBackend) Start(ctx context.Context) error {
	if b.binaryPath == "" || b.modelPath == "" {
		return fmt.Errorf("server or model not downloaded")
	}
	
	// Check if already running
	if b.process != nil && b.process.ProcessState == nil {
		return nil
	}
	
	// Prepare command arguments
	args := []string{
		"--model", b.modelPath,
		"--port", fmt.Sprintf("%d", b.port),
		"--host", "127.0.0.1",
		"--n-gpu-layers", "-1", // Use all GPU layers on Apple Silicon
		"--ctx-size", "8192",
		"--threads", fmt.Sprintf("%d", runtime.NumCPU()),
		"--mlock", // Lock model in memory
		"--no-mmap", // Don't use memory mapping for better performance
		"--jinja", // Enable jinja templating for tool support
	}
	
	// Add Apple Silicon optimizations
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		args = append(args, "--use-metal") // Enable Metal acceleration
	}
	
	// Start the server
	b.process = exec.CommandContext(ctx, b.binaryPath, args...)
	
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
		return fmt.Errorf("failed to start llama.cpp server: %w", err)
	}
	
	// Log output in background
	go b.logOutput(stdout, "stdout")
	go b.logOutput(stderr, "stderr")
	
	// Wait for server to be ready
	if err := b.waitForReady(ctx); err != nil {
		b.Stop()
		return fmt.Errorf("server failed to start: %w", err)
	}
	
	slog.Info("llama.cpp server started successfully", "port", b.port, "model", b.modelID)
	return nil
}

// Stop stops the llama.cpp server
func (b *LlamaCppBackend) Stop() error {
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
	slog.Info("llama.cpp server stopped")
	return nil
}

// GetEndpoint returns the OpenAI-compatible API endpoint
func (b *LlamaCppBackend) GetEndpoint() string {
	return fmt.Sprintf("http://localhost:%d/v1", b.port)
}

// IsRunning checks if the server is running
func (b *LlamaCppBackend) IsRunning() bool {
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

func (b *LlamaCppBackend) waitForReady(ctx context.Context) error {
	healthURL := fmt.Sprintf("http://localhost:%d/health", b.port)
	
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	timeout := time.After(30 * time.Second)
	
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
			resp.Body.Close()
			
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

func (b *LlamaCppBackend) logOutput(pipe io.Reader, name string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "error") || strings.Contains(line, "ERROR") {
			slog.Error("llama.cpp server output", "stream", name, "line", line)
		} else {
			slog.Debug("llama.cpp server output", "stream", name, "line", line)
		}
	}
}

// GetRecommendedGGUFModel returns the best GGUF model for the user's system
func GetRecommendedGGUFModel() ModelOption {
	// For 64GB M1 Max, recommend Qwen2.5-Coder-7B-Q4_K_M
	return ModelOption{
		ID:          "qwen2.5-coder-7b-q4_k_m",
		Name:        "Qwen 2.5 Coder 7B (Q4_K_M)",
		Description: "Best coding model in GGUF format. Optimized for Apple Silicon.",
		Size:        4794158596,              // 4.47GB actual size
		Memory:      8 * 1024 * 1024 * 1024, // 8GB RAM
		URL:         "https://huggingface.co/Qwen/Qwen2.5-Coder-7B-Instruct-GGUF/resolve/main/qwen2.5-coder-7b-instruct-q4_k_m.gguf",
		Provider:    "llamacpp",
	}
}