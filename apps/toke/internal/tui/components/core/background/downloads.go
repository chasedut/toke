package background

import (
	"fmt"
	"sync"
	
	"github.com/chasedut/toke/internal/backend"
)

// DownloadInfo represents a background download
type DownloadInfo struct {
	Model      *backend.ModelOption
	Progress   float64
	Downloaded int64
	Total      int64
	Complete   bool
	Error      error
}

// DownloadManager manages background downloads
type DownloadManager struct {
	mu        sync.RWMutex
	downloads map[string]*DownloadInfo // key is model ID
}

// NewDownloadManager creates a new download manager
func NewDownloadManager() *DownloadManager {
	return &DownloadManager{
		downloads: make(map[string]*DownloadInfo),
	}
}

// AddDownload adds a new background download
func (dm *DownloadManager) AddDownload(model *backend.ModelOption, progress float64, downloaded, total int64) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	dm.downloads[model.ID] = &DownloadInfo{
		Model:      model,
		Progress:   progress,
		Downloaded: downloaded,
		Total:      total,
		Complete:   false,
	}
}

// UpdateProgress updates the progress of a download
func (dm *DownloadManager) UpdateProgress(modelID string, progress float64, downloaded, total int64) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	if info, exists := dm.downloads[modelID]; exists {
		info.Progress = progress
		info.Downloaded = downloaded
		info.Total = total
	}
}

// CompleteDownload marks a download as complete
func (dm *DownloadManager) CompleteDownload(modelID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	if info, exists := dm.downloads[modelID]; exists {
		info.Complete = true
		info.Progress = 1.0
	}
}

// SetError sets an error for a download
func (dm *DownloadManager) SetError(modelID string, err error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	if info, exists := dm.downloads[modelID]; exists {
		info.Error = err
	}
}

// RemoveDownload removes a download from tracking
func (dm *DownloadManager) RemoveDownload(modelID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	delete(dm.downloads, modelID)
}

// GetActiveDownloads returns all active (non-complete) downloads
func (dm *DownloadManager) GetActiveDownloads() []*DownloadInfo {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	var active []*DownloadInfo
	for _, info := range dm.downloads {
		if !info.Complete && info.Error == nil {
			active = append(active, info)
		}
	}
	return active
}

// GetCompletedDownloads returns all completed downloads
func (dm *DownloadManager) GetCompletedDownloads() []*DownloadInfo {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	var completed []*DownloadInfo
	for _, info := range dm.downloads {
		if info.Complete {
			completed = append(completed, info)
		}
	}
	return completed
}

// HasActiveDownloads returns true if there are active downloads
func (dm *DownloadManager) HasActiveDownloads() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	for _, info := range dm.downloads {
		if !info.Complete && info.Error == nil {
			return true
		}
	}
	return false
}

// GetStatusString returns a string representation of download status
func (dm *DownloadManager) GetStatusString() string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	active := 0
	var currentDownload *DownloadInfo
	
	for _, info := range dm.downloads {
		if !info.Complete && info.Error == nil {
			active++
			if currentDownload == nil || info.Progress > currentDownload.Progress {
				currentDownload = info
			}
		}
	}
	
	if active == 0 {
		return ""
	}
	
	if currentDownload != nil {
		percent := int(currentDownload.Progress * 100)
		return fmt.Sprintf("📥 Downloading %s... %d%% (press 'b' to view)", currentDownload.Model.Name, percent)
	}
	
	return fmt.Sprintf("📥 %d downloads in progress (press 'b' to view)", active)
}