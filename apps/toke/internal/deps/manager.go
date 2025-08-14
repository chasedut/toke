package deps

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Dependency struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	Executable  string `json:"executable"`
	LocalPath   string `json:"local_path,omitempty"`
	Required    bool   `json:"required"`
	Platform    string `json:"platform,omitempty"`
}

type Manifest struct {
	Version      string       `json:"version"`
	Dependencies []Dependency `json:"dependencies"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type Manager struct {
	baseDir      string
	manifestURL  string
	githubRepo   string
	progressChan chan ProgressUpdate
	localPaths   map[string]string // Map of dependency name to local build path
}

type ProgressUpdate struct {
	Name         string
	CurrentBytes int64
	TotalBytes   int64
	Status       string
	Error        error
}

func NewManager(baseDir, githubRepo string) *Manager {
	// Normalize architecture naming
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"  // Keep as-is for consistency
	} else if arch == "386" {
		arch = "386"
	}

	// Build platform-specific executable names
	var llamaExe, ngrokExe string
	if runtime.GOOS == "windows" {
		llamaExe = fmt.Sprintf("llama-server-%s-%s.exe", runtime.GOOS, arch)
		ngrokExe = "ngrok.exe"
	} else {
		llamaExe = fmt.Sprintf("llama-server-%s-%s", runtime.GOOS, arch)
		ngrokExe = "ngrok"
	}

	return &Manager{
		baseDir:      baseDir,
		githubRepo:   githubRepo,
		manifestURL:  fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo),
		progressChan: make(chan ProgressUpdate, 100),
		localPaths: map[string]string{
			"llama":     filepath.Join("apps", "backend", "llama", llamaExe),
			"mlx":       "apps/backend/mlx/mlx-server",
			"diffusion": "apps/backend/diffusion/diffusion_server.py",
			"ngrok":     filepath.Join("apps", "ngrok", "node_modules", ".bin", ngrokExe),
		},
	}
}

func (m *Manager) GetProgressChannel() <-chan ProgressUpdate {
	return m.progressChan
}

func (m *Manager) checkLocalBuild(dep Dependency) (string, bool) {
	// Try multiple possible local paths for the dependency
	possiblePaths := []string{}
	
	// Add configured local path
	if localPath, exists := m.localPaths[dep.Name]; exists {
		possiblePaths = append(possiblePaths, localPath)
	}
	
	// Add platform-specific variations
	if dep.Name == "llama" {
		// Try different naming conventions
		possiblePaths = append(possiblePaths,
			filepath.Join("apps", "backend", "llama", "llama-server"),
			filepath.Join("apps", "backend", "llama", fmt.Sprintf("llama-server-%s-%s", runtime.GOOS, runtime.GOARCH)),
			filepath.Join("build-llama-server", "llama-server"),
			filepath.Join("build-llama-server", fmt.Sprintf("llama-server-%s-%s", runtime.GOOS, runtime.GOARCH)),
		)
		if runtime.GOOS == "windows" {
			possiblePaths = append(possiblePaths,
				filepath.Join("apps", "backend", "llama", "llama-server.exe"),
				filepath.Join("apps", "backend", "llama", fmt.Sprintf("llama-server-%s-%s.exe", runtime.GOOS, runtime.GOARCH)),
			)
		}
	} else if dep.Name == "ngrok" {
		possiblePaths = append(possiblePaths,
			filepath.Join("apps", "ngrok", "ngrok"),
			filepath.Join("ngrok"),
		)
		if runtime.GOOS == "windows" {
			possiblePaths = append(possiblePaths,
				filepath.Join("apps", "ngrok", "ngrok.exe"),
				"ngrok.exe",
			)
		}
	}

	// Add common system locations
	possiblePaths = append(possiblePaths,
		filepath.Join(os.Getenv("HOME"), ".toke", "backends", dep.Name, dep.Executable),
		filepath.Join("/usr/local/bin", dep.Executable),
		filepath.Join("/opt/toke", dep.Name, dep.Executable),
	)
	
	// Windows-specific paths
	if runtime.GOOS == "windows" {
		possiblePaths = append(possiblePaths,
			filepath.Join(os.Getenv("PROGRAMFILES"), "toke", dep.Name, dep.Executable),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "toke", "backends", dep.Name, dep.Executable),
		)
	}

	// Check each possible path
	for _, path := range possiblePaths {
		if path != "" && fileExists(path) {
			m.sendProgress(dep.Name, 0, 0, fmt.Sprintf("Using local build: %s", path), nil)
			return path, true
		}
	}

	return "", false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func (m *Manager) Initialize() error {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(m.baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create base directory: %w", err)
	}

	return nil
}

func (m *Manager) GetManifest(ctx context.Context) (*Manifest, error) {
	// Check for embedded manifest first (in the binary)
	embeddedManifestPath := filepath.Join(filepath.Dir(os.Args[0]), "backends-manifest.json")
	if data, err := os.ReadFile(embeddedManifestPath); err == nil {
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err == nil {
			return &manifest, nil
		}
	}

	// Then check for local cached manifest
	manifestPath := filepath.Join(m.baseDir, "manifest.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err == nil {
			// Check if manifest is less than 24 hours old
			if time.Since(manifest.UpdatedAt) < 24*time.Hour {
				return &manifest, nil
			}
		}
	}

	// Fetch latest manifest from GitHub
	manifest, err := m.fetchLatestManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Save manifest locally
	if data, err := json.MarshalIndent(manifest, "", "  "); err == nil {
		os.WriteFile(manifestPath, data, 0644)
	}

	return manifest, nil
}

func (m *Manager) fetchLatestManifest(ctx context.Context) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", m.manifestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			Size               int    `json:"size"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	// Build manifest from release assets
	manifest := &Manifest{
		Version:   release.TagName,
		UpdatedAt: time.Now(),
	}

	// Normalize platform naming
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	} else if arch == "386" {
		arch = "386"  // Windows 32-bit
	}
	
	platform := runtime.GOOS + "-" + arch
	
	// Platform-specific executable names
	var llamaExe, ngrokExe string
	if runtime.GOOS == "windows" {
		llamaExe = "llama-server.exe"
		ngrokExe = "ngrok.exe"
	} else {
		llamaExe = "llama-server"
		ngrokExe = "ngrok"
	}

	// Define dependencies based on platform
	deps := []struct {
		name       string
		executable string
		pattern    string
		required   bool
	}{
		{"llama", llamaExe, "llama-server-" + platform, true},
		{"mlx", "mlx-server", "mlx-server-bundle", runtime.GOOS == "darwin" && arch == "arm64"},
		{"diffusion", "diffusion_server.py", "diffusion-server-bundle", false},
		{"ngrok", ngrokExe, "ngrok-" + platform, false},
	}

	for _, dep := range deps {
		for _, asset := range release.Assets {
			if strings.Contains(asset.Name, dep.pattern) {
				manifest.Dependencies = append(manifest.Dependencies, Dependency{
					Name:       dep.name,
					Version:    release.TagName,
					URL:        asset.BrowserDownloadURL,
					Size:       int64(asset.Size),
					Executable: dep.executable,
					Required:   dep.required,
					Platform:   platform,
				})
				break
			}
		}
	}

	return manifest, nil
}

func (m *Manager) CheckAndInstall(ctx context.Context) error {
	manifest, err := m.GetManifest(ctx)
	if err != nil {
		return err
	}

	for _, dep := range manifest.Dependencies {
		// Check for local build first
		if localPath, exists := m.checkLocalBuild(dep); exists {
			dep.LocalPath = localPath
			continue
		}

		// Check if already installed
		installPath := filepath.Join(m.baseDir, dep.Name, dep.Executable)
		if _, err := os.Stat(installPath); err == nil {
			m.sendProgress(dep.Name, 0, 0, "Already installed", nil)
			continue
		}

		// Download and install
		if err := m.installDependency(ctx, dep); err != nil {
			if dep.Required {
				return fmt.Errorf("failed to install required dependency %s: %w", dep.Name, err)
			}
			m.sendProgress(dep.Name, 0, 0, "Skipped (optional)", err)
		}
	}

	return nil
}

func (m *Manager) installDependency(ctx context.Context, dep Dependency) error {
	m.sendProgress(dep.Name, 0, dep.Size, "Starting download", nil)

	// Create directory for dependency
	depDir := filepath.Join(m.baseDir, dep.Name)
	if err := os.MkdirAll(depDir, 0755); err != nil {
		return err
	}

	// Download file
	req, err := http.NewRequestWithContext(ctx, "GET", dep.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create temporary file
	tmpFile, err := os.CreateTemp(depDir, "download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	// Download with progress
	reader := &progressReader{
		Reader:   resp.Body,
		Total:    dep.Size,
		Current:  0,
		Callback: func(current, total int64) {
			m.sendProgress(dep.Name, current, total, "Downloading", nil)
		},
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		return err
	}
	tmpFile.Close()

	// Extract if it's an archive
	if strings.HasSuffix(dep.URL, ".tar.gz") || strings.HasSuffix(dep.URL, ".tgz") {
		m.sendProgress(dep.Name, dep.Size, dep.Size, "Extracting", nil)
		if err := m.extractTarGz(tmpFile.Name(), depDir); err != nil {
			return err
		}
	} else if strings.HasSuffix(dep.URL, ".zip") {
		m.sendProgress(dep.Name, dep.Size, dep.Size, "Extracting", nil)
		if err := m.extractZip(tmpFile.Name(), depDir); err != nil {
			return err
		}
	} else {
		// Just move the file
		finalPath := filepath.Join(depDir, dep.Executable)
		if err := os.Rename(tmpFile.Name(), finalPath); err != nil {
			return err
		}
	}

	// Find the actual executable that was extracted
	// The archive might contain the file with a different name than expected
	possibleExecs := []string{
		dep.Executable,
		"llama-server",
		"llama-server.exe",
		"ngrok",
		"ngrok.exe",
		fmt.Sprintf("llama-server-%s-%s", runtime.GOOS, runtime.GOARCH),
		fmt.Sprintf("llama-server-%s-%s.exe", runtime.GOOS, runtime.GOARCH),
	}
	
	var actualExecPath string
	for _, exec := range possibleExecs {
		testPath := filepath.Join(depDir, exec)
		if fileExists(testPath) {
			actualExecPath = testPath
			// Rename to expected name if different
			if exec != dep.Executable {
				expectedPath := filepath.Join(depDir, dep.Executable)
				os.Rename(testPath, expectedPath)
				actualExecPath = expectedPath
			}
			break
		}
	}
	
	if actualExecPath == "" {
		return fmt.Errorf("could not find executable after extraction for %s", dep.Name)
	}

	// Make executable (not needed on Windows but doesn't hurt)
	if err := os.Chmod(actualExecPath, 0755); err != nil {
		// Ignore chmod errors on Windows
		if runtime.GOOS != "windows" {
			return err
		}
	}

	// Write version file
	versionFile := filepath.Join(depDir, "VERSION")
	if err := os.WriteFile(versionFile, []byte(dep.Version), 0644); err != nil {
		// Non-fatal error, just log it
		m.sendProgress(dep.Name, 0, 0, "Warning: Could not write version file", err)
	}

	m.sendProgress(dep.Name, dep.Size, dep.Size, "Installed", nil)
	return nil
}

func (m *Manager) extractTarGz(src, dst string) error {
	file, err := os.Open(src)
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

		target := filepath.Join(dst, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			file, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return err
			}
			file.Close()
		}
	}

	return nil
}

func (m *Manager) extractZip(src, dst string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(dst, file.Name)

		// Check for ZipSlip vulnerability
		if !strings.HasPrefix(path, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.Mode())
			continue
		}

		fileReader, err := file.Open()
		if err != nil {
			return err
		}
		defer fileReader.Close()

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}
		defer targetFile.Close()

		if _, err := io.Copy(targetFile, fileReader); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) CheckForUpdates(ctx context.Context) ([]Dependency, error) {
	manifest, err := m.GetManifest(ctx)
	if err != nil {
		return nil, err
	}

	var updates []Dependency

	for _, dep := range manifest.Dependencies {
		// Check local build first - skip update if using local
		if _, exists := m.checkLocalBuild(dep); exists {
			m.sendProgress(dep.Name, 0, 0, "Using local build, skipping update check", nil)
			continue
		}

		// Check installed version
		versionFile := filepath.Join(m.baseDir, dep.Name, "VERSION")
		installedVersionBytes, err := os.ReadFile(versionFile)
		if err != nil {
			// Check for executable to see if it's installed without version
			execPath := filepath.Join(m.baseDir, dep.Name, dep.Executable)
			if fileExists(execPath) {
				// Installed but no version file - needs update
				updates = append(updates, dep)
				m.sendProgress(dep.Name, 0, 0, "No version file, update available", nil)
			}
			continue
		}

		installedVersion := strings.TrimSpace(string(installedVersionBytes))
		if installedVersion != dep.Version {
			updates = append(updates, dep)
			m.sendProgress(dep.Name, 0, 0, fmt.Sprintf("Update available: %s -> %s", installedVersion, dep.Version), nil)
		} else {
			m.sendProgress(dep.Name, 0, 0, fmt.Sprintf("Up to date: %s", installedVersion), nil)
		}
	}

	return updates, nil
}

func (m *Manager) GetExecutablePath(ctx context.Context, name string) (string, error) {
	// Check for local build first
	if localPath, exists := m.localPaths[name]; exists {
		if _, err := os.Stat(localPath); err == nil {
			return localPath, nil
		}
	}

	// Check installed path with platform-specific executable name
	var executable string
	switch name {
	case "llama":
		if runtime.GOOS == "windows" {
			executable = "llama-server.exe"
		} else {
			executable = "llama-server"
		}
	case "mlx":
		executable = "mlx_server.py"
	case "diffusion":
		executable = "diffusion_server.py"
	case "ngrok":
		if runtime.GOOS == "windows" {
			executable = "ngrok.exe"
		} else {
			executable = "ngrok"
		}
	default:
		// Try to get from manifest
		manifest, err := m.GetManifest(ctx)
		if err != nil {
			return "", err
		}
		for _, dep := range manifest.Dependencies {
			if dep.Name == name {
				executable = dep.Executable
				break
			}
		}
	}

	// Check if installed in base directory
	installedPath := filepath.Join(m.baseDir, name, executable)
	if _, err := os.Stat(installedPath); err == nil {
		return installedPath, nil
	}

	return "", fmt.Errorf("dependency %s not found", name)
}

func (m *Manager) GetInstalledVersions(ctx context.Context) map[string]string {
	versions := make(map[string]string)
	
	manifest, err := m.GetManifest(ctx)
	if err != nil {
		return versions
	}
	
	for _, dep := range manifest.Dependencies {
		// Check if using local build
		if _, exists := m.checkLocalBuild(dep); exists {
			// Try to read local version file
			localVersionPaths := []string{
				filepath.Join("apps", "backend", dep.Name, "VERSION"),
				filepath.Join("apps", dep.Name, "VERSION"),
			}
			for _, path := range localVersionPaths {
				if data, err := os.ReadFile(path); err == nil {
					versions[dep.Name] = strings.TrimSpace(string(data)) + " (local)"
					break
				}
			}
			if _, exists := versions[dep.Name]; !exists {
				versions[dep.Name] = "local build"
			}
			continue
		}
		
		// Check installed version
		versionFile := filepath.Join(m.baseDir, dep.Name, "VERSION")
		if data, err := os.ReadFile(versionFile); err == nil {
			versions[dep.Name] = strings.TrimSpace(string(data))
		} else {
			// Check if installed without version
			execPath := filepath.Join(m.baseDir, dep.Name, dep.Executable)
			if fileExists(execPath) {
				versions[dep.Name] = "unknown"
			} else {
				versions[dep.Name] = "not installed"
			}
		}
	}
	
	return versions
}

func (m *Manager) sendProgress(name string, current, total int64, status string, err error) {
	select {
	case m.progressChan <- ProgressUpdate{
		Name:         name,
		CurrentBytes: current,
		TotalBytes:   total,
		Status:       status,
		Error:        err,
	}:
	default:
		// Channel full, skip update
	}
}

type progressReader struct {
	io.Reader
	Total    int64
	Current  int64
	Callback func(current, total int64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.Current += int64(n)
	if r.Callback != nil {
		r.Callback(r.Current, r.Total)
	}
	return n, err
}