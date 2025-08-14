package update

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

	"github.com/Masterminds/semver/v3"
)

const (
	githubAPIURL = "https://api.github.com/repos/chasedut/toke/releases/latest"
	githubRepo   = "chasedut/toke"
)

type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
	HTMLURL     string    `json:"html_url"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type UpdateInfo struct {
	Available       bool
	CurrentVersion  string
	LatestVersion   string
	ReleaseNotes    string
	ReleaseURL      string
	DownloadURL     string
	AssetSize       int64
	PublishedAt     time.Time
}

type Updater struct {
	currentVersion string
	httpClient     *http.Client
}

func New(currentVersion string) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (u *Updater) CheckForUpdate(ctx context.Context) (*UpdateInfo, error) {
	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}

	currentVer, err := semver.NewVersion(strings.TrimPrefix(u.currentVersion, "v"))
	if err != nil {
		return nil, fmt.Errorf("invalid current version %s: %w", u.currentVersion, err)
	}

	latestVer, err := semver.NewVersion(strings.TrimPrefix(release.TagName, "v"))
	if err != nil {
		return nil, fmt.Errorf("invalid latest version %s: %w", release.TagName, err)
	}

	info := &UpdateInfo{
		CurrentVersion: u.currentVersion,
		LatestVersion:  release.TagName,
		ReleaseNotes:   release.Body,
		ReleaseURL:     release.HTMLURL,
		PublishedAt:    release.PublishedAt,
	}

	if latestVer.GreaterThan(currentVer) {
		info.Available = true
		
		assetName := u.getAssetName()
		for _, asset := range release.Assets {
			if asset.Name == assetName {
				info.DownloadURL = asset.BrowserDownloadURL
				info.AssetSize = asset.Size
				break
			}
		}
		
		if info.DownloadURL == "" {
			return nil, fmt.Errorf("no compatible binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
		}
	}

	return info, nil
}

func (u *Updater) fetchLatestRelease(ctx context.Context) (*GitHubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubAPIURL, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}
	
	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	
	return &release, nil
}

func (u *Updater) getAssetName() string {
	var assetName string
	
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			assetName = "toke-darwin-arm64.tar.gz"
		} else {
			assetName = "toke-darwin-amd64.tar.gz"
		}
	case "linux":
		assetName = "toke-linux-amd64.tar.gz"
	case "windows":
		assetName = "toke-windows-amd64.zip"
	default:
		assetName = fmt.Sprintf("toke-%s-%s", runtime.GOOS, runtime.GOARCH)
	}
	
	return assetName
}

func (u *Updater) DownloadAndInstall(ctx context.Context, updateInfo *UpdateInfo, showProgress func(current, total int64)) error {
	if !updateInfo.Available || updateInfo.DownloadURL == "" {
		return fmt.Errorf("no update available")
	}

	tempDir, err := os.MkdirTemp("", "toke-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	downloadPath := filepath.Join(tempDir, filepath.Base(updateInfo.DownloadURL))
	if err := u.downloadFile(ctx, updateInfo.DownloadURL, downloadPath, showProgress); err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}

	extractedPath := filepath.Join(tempDir, "extracted")
	if err := os.MkdirAll(extractedPath, 0755); err != nil {
		return fmt.Errorf("failed to create extraction directory: %w", err)
	}

	if err := u.extractArchive(downloadPath, extractedPath); err != nil {
		return fmt.Errorf("failed to extract update: %w", err)
	}

	binaryName := "toke"
	if runtime.GOOS == "windows" {
		binaryName = "toke.exe"
	}
	
	newBinaryPath := filepath.Join(extractedPath, binaryName)
	if _, err := os.Stat(newBinaryPath); os.IsNotExist(err) {
		files, _ := filepath.Glob(filepath.Join(extractedPath, "*"))
		if len(files) == 1 && strings.Contains(files[0], "toke") {
			newBinaryPath = files[0]
		} else {
			return fmt.Errorf("binary not found in extracted archive")
		}
	}

	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable path: %w", err)
	}

	backupPath := currentBinary + ".backup"
	if err := os.Rename(currentBinary, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	if err := u.copyFile(newBinaryPath, currentBinary); err != nil {
		os.Rename(backupPath, currentBinary)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	if err := os.Chmod(currentBinary, 0755); err != nil {
		os.Remove(currentBinary)
		os.Rename(backupPath, currentBinary)
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	os.Remove(backupPath)
	
	return nil
}

func (u *Updater) downloadFile(ctx context.Context, url, dest string, showProgress func(current, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if showProgress != nil {
		reader := &progressReader{
			Reader:   resp.Body,
			Total:    resp.ContentLength,
			Callback: showProgress,
		}
		_, err = io.Copy(out, reader)
	} else {
		_, err = io.Copy(out, resp.Body)
	}

	return err
}

func (u *Updater) extractArchive(archivePath, destPath string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return u.extractZip(archivePath, destPath)
	} else if strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		return u.extractTarGz(archivePath, destPath)
	}
	return fmt.Errorf("unsupported archive format")
}

func (u *Updater) extractZip(zipPath, destPath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(destPath, file.Name)
		
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

		_, err = io.Copy(targetFile, fileReader)
		if err != nil {
			return err
		}
	}

	return nil
}

func (u *Updater) extractTarGz(tarGzPath, destPath string) error {
	file, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(destPath, header.Name)
		
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			
			outFile, err := os.Create(path)
			if err != nil {
				return err
			}
			
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
			
			if err := os.Chmod(path, os.FileMode(header.Mode)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (u *Updater) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
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