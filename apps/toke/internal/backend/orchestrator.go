package backend

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// ModelBackend interface for different model providers
type ModelBackend interface {
	DownloadServer(ctx context.Context, progressFn func(downloaded, total int64)) error
	DownloadModel(ctx context.Context, model ModelOption, progressFn func(downloaded, total int64)) error
	Start(ctx context.Context) error
	Stop() error
	GetEndpoint() string
	IsRunning() bool
}

// Orchestrator manages the complete local AI backend lifecycle
type Orchestrator struct {
	dataDir      string
	backend      ModelBackend
	model        *ModelOption
	mu           sync.Mutex
	isRunning    bool
	downloadFunc func(ctx context.Context, progress func(downloaded, total int64)) error
}

// NewOrchestrator creates a new backend orchestrator
func NewOrchestrator(dataDir string) *Orchestrator {
	return &Orchestrator{
		dataDir: dataDir,
	}
}

// SetupModel downloads and configures the specified model
func (o *Orchestrator) SetupModel(ctx context.Context, model *ModelOption, progressFn func(downloaded, total int64)) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	o.model = model
	
	// Create data directories
	if err := o.ensureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	
	// Create appropriate backend based on provider
	var backend ModelBackend
	switch model.Provider {
	case "mlx":
		backend = NewMLXBackend(o.dataDir, model.ID)
	case "llamacpp":
		backend = NewLlamaCppBackend(o.dataDir, model.ID)
	default:
		return fmt.Errorf("unsupported provider: %s", model.Provider)
	}
	
	// Step 1: Download server if needed
	slog.Info("Checking server installation...", "provider", model.Provider)
	if err := backend.DownloadServer(ctx, func(downloaded, total int64) {
		// Report server download progress (small, so scale it down)
		scaledProgress := downloaded / 10 // Server is smaller than models
		scaledTotal := total / 10
		progressFn(scaledProgress, model.Size+scaledTotal)
	}); err != nil {
		return fmt.Errorf("failed to download server: %w", err)
	}
	
	// Step 2: Download model if needed
	slog.Info("Downloading model", "model", model.Name, "size", FormatSize(model.Size))
	if err := backend.DownloadModel(ctx, *model, progressFn); err != nil {
		return fmt.Errorf("failed to download model: %w", err)
	}
	
	// Step 3: Store backend instance
	o.backend = backend
	
	return nil
}

// Start starts the backend server
func (o *Orchestrator) Start(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	if o.isRunning {
		return nil
	}
	
	if o.backend == nil {
		return fmt.Errorf("backend not initialized, call SetupModel first")
	}
	
	slog.Info("Starting local AI backend...")
	
	if err := o.backend.Start(ctx); err != nil {
		return fmt.Errorf("failed to start backend: %w", err)
	}
	
	o.isRunning = true
	
	// Start health monitor
	go o.monitorHealth(ctx)
	
	return nil
}

// Stop stops the backend server
func (o *Orchestrator) Stop() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	if !o.isRunning {
		return nil
	}
	
	if o.backend != nil {
		if err := o.backend.Stop(); err != nil {
			return fmt.Errorf("failed to stop backend: %w", err)
		}
	}
	
	o.isRunning = false
	return nil
}

// GetEndpoint returns the API endpoint if running
func (o *Orchestrator) GetEndpoint() (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	if !o.isRunning || o.backend == nil {
		return "", fmt.Errorf("backend not running")
	}
	
	return o.backend.GetEndpoint(), nil
}

// IsRunning checks if the backend is running
func (o *Orchestrator) IsRunning() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	return o.isRunning && o.backend != nil && o.backend.IsRunning()
}

// GetModel returns the currently configured model
func (o *Orchestrator) GetModel() *ModelOption {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	return o.model
}

// CheckSystemRequirements verifies the system can run local models
func (o *Orchestrator) CheckSystemRequirements() error {
	// Check platform - MLX requires Apple Silicon, llama.cpp is more flexible
	if runtime.GOOS == "darwin" && runtime.GOARCH != "arm64" {
		// Intel Macs can still use llama.cpp but not MLX
		slog.Warn("Intel Mac detected - MLX models not available, only GGUF models supported")
	}
	
	// Check available memory
	// TODO: Implement actual memory check
	
	return nil
}

// QuickSetup performs a complete setup with the recommended model
func (o *Orchestrator) QuickSetup(ctx context.Context, progressFn func(status string, downloaded, total int64)) error {
	// Check system requirements
	if err := o.CheckSystemRequirements(); err != nil {
		return err
	}
	
	// Get recommended model based on platform
	var model *ModelOption
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		// On Apple Silicon, prefer MLX models
		for _, m := range AvailableModels() {
			if m.Provider == "mlx" && m.Recommended {
				model = &m
				break
			}
		}
	}
	
	// Fallback to default recommended model
	if model == nil {
		model = GetRecommendedModel()
	}
	
	if model == nil {
		return fmt.Errorf("no recommended model available")
	}
	
	// Report starting
	progressFn("Preparing setup...", 0, model.Size)
	
	// Setup the model
	if err := o.SetupModel(ctx, model, func(downloaded, total int64) {
		status := "Downloading model..."
		if downloaded == total {
			status = "Model ready!"
		}
		progressFn(status, downloaded, total)
	}); err != nil {
		return err
	}
	
	// Start the backend
	progressFn("Starting AI server...", model.Size, model.Size)
	if err := o.Start(ctx); err != nil {
		return err
	}
	
	// Verify it's working
	progressFn("Verifying connection...", model.Size, model.Size)
	if !o.waitForReady(ctx, 30*time.Second) {
		return fmt.Errorf("backend failed to start properly")
	}
	
	progressFn("Ready!", model.Size, model.Size)
	return nil
}

// Private methods

func (o *Orchestrator) ensureDirectories() error {
	dirs := []string{
		filepath.Join(o.dataDir, "bin"),
		filepath.Join(o.dataDir, "models"),
		filepath.Join(o.dataDir, "cache"),
	}
	
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	
	return nil
}

func (o *Orchestrator) monitorHealth(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !o.backend.IsRunning() {
				slog.Warn("Backend stopped unexpectedly, attempting restart...")
				if err := o.backend.Start(ctx); err != nil {
					slog.Error("Failed to restart backend", "error", err)
				}
			}
		}
	}
}

func (o *Orchestrator) waitForReady(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if time.Now().After(deadline) {
				return false
			}
			if o.backend.IsRunning() {
				return true
			}
		}
	}
}