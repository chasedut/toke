package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HFFile represents a file in the HuggingFace repository
type HFFile struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	LFS  struct {
		Size int64 `json:"size"`
	} `json:"lfs"`
}

// DownloadMLXModel downloads all necessary files for an MLX model
func (b *MLXBackend) DownloadMLXModel(ctx context.Context, model ModelOption, progressFn func(downloaded, total int64)) error {
	modelPath := filepath.Join(b.dataDir, "models", "mlx", model.ID)
	
	// Quick check if already downloaded
	if b.isModelComplete(modelPath) {
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
	
	// Get list of files from HuggingFace API
	slog.Info("Getting file list for MLX model", "url", model.URL)
	files, err := b.getModelFiles(ctx, model.URL)
	if err != nil {
		slog.Warn("Failed to get file list from API, using fallback", "error", err, "url", model.URL)
		// Use fallback file list
		files = b.getFallbackFiles(model.URL)
	}
	
	slog.Info("Found files for MLX model", "count", len(files))
	
	// Filter to only necessary files
	necessaryFiles := b.filterNecessaryFiles(files)
	slog.Info("Filtered to necessary files", "count", len(necessaryFiles))
	
	// If no files found, return error
	if len(necessaryFiles) == 0 {
		return fmt.Errorf("no model files found to download")
	}
	
	// Calculate total size
	totalSize := int64(0)
	for _, file := range necessaryFiles {
		totalSize += file.Size
	}
	if totalSize == 0 {
		totalSize = model.Size // Use estimated size if we can't calculate
	}
	slog.Info("Total download size", "size", totalSize)
	
	// Report initial progress
	if progressFn != nil && totalSize > 0 {
		progressFn(0, totalSize)
	}
	
	// Download files
	totalDownloaded := int64(0)
	for _, file := range necessaryFiles {
		filePath := filepath.Join(modelPath, file.Path)
		
		// Skip if file exists and has correct size
		if info, err := os.Stat(filePath); err == nil && info.Size() == file.Size {
			totalDownloaded += file.Size
			if progressFn != nil {
				progressFn(totalDownloaded, totalSize)
			}
			continue
		}
		
		// Download file with progress
		fileURL := fmt.Sprintf("%s/resolve/main/%s", model.URL, file.Path)
		slog.Info("Downloading model file", "file", file.Path, "size", file.Size)
		
		// Create a progress callback for this specific file
		fileProgressFn := func(fileDownloaded int64) {
			if progressFn != nil {
				progressFn(totalDownloaded + fileDownloaded, totalSize)
			}
		}
		
		downloaded, err := b.downloadFileWithProgress(ctx, fileURL, filePath, fileProgressFn)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", file.Path, err)
		}
		
		totalDownloaded += downloaded
		if progressFn != nil {
			progressFn(totalDownloaded, totalSize)
		}
	}
	
	b.modelPath = modelPath
	slog.Info("MLX model downloaded successfully", "path", modelPath)
	return nil
}

func (b *MLXBackend) getModelFiles(ctx context.Context, modelURL string) ([]HFFile, error) {
	// Convert model URL to API URL
	apiURL := strings.Replace(modelURL, "https://huggingface.co/", "https://huggingface.co/api/models/", 1) + "/tree/main"
	
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		// Read error body for debugging
		body, _ := io.ReadAll(resp.Body)
		slog.Error("API request failed", "status", resp.StatusCode, "url", apiURL, "response", string(body))
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}
	
	var files []HFFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}
	
	// Update sizes from LFS if needed
	for i := range files {
		if files[i].LFS.Size > 0 {
			files[i].Size = files[i].LFS.Size
		}
	}
	
	return files, nil
}

func (b *MLXBackend) getFallbackFiles(modelURL string) []HFFile {
	// Return common MLX model files as fallback
	// For MLX models, we typically have weight files split into parts
	files := []HFFile{
		{Path: "config.json", Size: 10000},
		{Path: "tokenizer.json", Size: 1000000},
		{Path: "tokenizer_config.json", Size: 10000},
		{Path: "special_tokens_map.json", Size: 5000},
		{Path: "model.safetensors.index.json", Size: 50000},
	}
	
	// Add weight files (MLX models are often split into multiple parts)
	// Estimate based on typical MLX model structure
	if strings.Contains(modelURL, "7B") {
		// For 7B models, typically 2-4 parts
		partSize := int64(2 * 1024 * 1024 * 1024) // 2GB per part
		files = append(files, 
			HFFile{Path: "model-00001-of-00003.safetensors", Size: partSize},
			HFFile{Path: "model-00002-of-00003.safetensors", Size: partSize},
			HFFile{Path: "model-00003-of-00003.safetensors", Size: partSize},
		)
	} else {
		// Default single weight file
		files = append(files, HFFile{Path: "model.safetensors", Size: 5 * 1024 * 1024 * 1024})
	}
	
	return files
}

func (b *MLXBackend) filterNecessaryFiles(files []HFFile) []HFFile {
	necessary := []HFFile{}
	
	for _, file := range files {
		// Skip README and other non-essential files
		if strings.HasSuffix(file.Path, ".md") || 
		   strings.HasSuffix(file.Path, ".txt") ||
		   strings.HasPrefix(file.Path, ".") {
			continue
		}
		
		// Include config files
		if strings.HasSuffix(file.Path, ".json") {
			necessary = append(necessary, file)
			continue
		}
		
		// Include weight files (.safetensors)
		if strings.HasSuffix(file.Path, ".safetensors") {
			necessary = append(necessary, file)
			continue
		}
		
		// Include tokenizer files
		if strings.Contains(file.Path, "tokenizer") {
			necessary = append(necessary, file)
		}
	}
	
	return necessary
}

// progressWriter wraps an io.Writer to report progress
type progressWriter struct {
	writer      io.Writer
	total       int64
	written     int64
	progressFn  func(written int64)
	lastReport  time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.written += int64(n)
	
	// Report progress at most once per 100ms to avoid overwhelming the UI
	if time.Since(pw.lastReport) > 100*time.Millisecond {
		if pw.progressFn != nil {
			pw.progressFn(pw.written)
		}
		pw.lastReport = time.Now()
	}
	
	return n, err
}

func (b *MLXBackend) downloadFileWithProgress(ctx context.Context, url, filePath string, progressFn func(int64)) (int64, error) {
	slog.Info("Starting file download with progress", "url", url, "path", filePath)
	
	// Check if file directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Create the file
	out, err := os.Create(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer out.Close()
	
	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers to avoid rate limiting
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	
	// Do request
	client := &http.Client{
		Timeout: 30 * time.Minute, // Large timeout for big files
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Handle redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("Download failed", "status", resp.StatusCode, "url", url, "response", string(body))
		return 0, fmt.Errorf("server returned status %d", resp.StatusCode)
	}
	
	slog.Info("Download response received", "status", resp.StatusCode, "contentLength", resp.ContentLength)
	
	// Create progress writer
	pw := &progressWriter{
		writer:     out,
		total:      resp.ContentLength,
		progressFn: progressFn,
		lastReport: time.Now(),
	}
	
	// Copy data with progress
	written, err := io.Copy(pw, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to copy data: %w", err)
	}
	
	// Final progress report
	if progressFn != nil {
		progressFn(written)
	}
	
	slog.Info("File downloaded successfully", "path", filePath, "size", written)
	return written, nil
}

// Keep the old function for compatibility
func (b *MLXBackend) downloadFile(ctx context.Context, url, filePath string) (int64, error) {
	return b.downloadFileWithProgress(ctx, url, filePath, nil)
}

func (b *MLXBackend) isModelComplete(modelPath string) bool {
	// Check for essential files
	essentialFiles := []string{
		"config.json",
		"tokenizer.json",
	}
	
	for _, file := range essentialFiles {
		if _, err := os.Stat(filepath.Join(modelPath, file)); err != nil {
			return false
		}
	}
	
	// Check for weight files (either single or multi-part)
	hasWeights := false
	
	// Check for single weight file
	if _, err := os.Stat(filepath.Join(modelPath, "model.safetensors")); err == nil {
		hasWeights = true
	}
	
	// Check for multi-part weight files (check for first part)
	if !hasWeights {
		pattern := filepath.Join(modelPath, "model-00001-of-*.safetensors")
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			hasWeights = true
		}
	}
	
	return hasWeights
}