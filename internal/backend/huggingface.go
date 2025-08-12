package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// HuggingFaceModel represents a model from Hugging Face
type HuggingFaceModel struct {
	ID           string    `json:"id"`
	Author       string    `json:"author"`
	ModelID      string    `json:"modelId"`
	Downloads    int       `json:"downloads"`
	Likes        int       `json:"likes"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"createdAt"`
	LastModified time.Time `json:"lastModified"`
	Private      bool      `json:"private"`
	Gated        bool      `json:"gated"`
	Disabled     bool      `json:"disabled"`
	LibraryName  string    `json:"library_name"`
	PipelineTag  string    `json:"pipeline_tag"`
}

// HuggingFaceFile represents a file in a HuggingFace repo
type HuggingFaceFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	LFS  struct {
		Size int64 `json:"size"`
	} `json:"lfs"`
}

// HuggingFaceClient provides access to the Hugging Face API
type HuggingFaceClient struct {
	baseURL string
	client  *http.Client
}

// NewHuggingFaceClient creates a new Hugging Face API client
func NewHuggingFaceClient() *HuggingFaceClient {
	return &HuggingFaceClient{
		baseURL: "https://huggingface.co/api",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SearchModels searches for models on Hugging Face
func (h *HuggingFaceClient) SearchModels(ctx context.Context, query string, filter string) ([]HuggingFaceModel, error) {
	params := url.Values{}
	params.Set("search", query)
	params.Set("sort", "downloads")
	params.Set("direction", "-1")
	params.Set("limit", "50")
	
	// Add filter if specified
	if filter != "" {
		params.Set("filter", filter)
	}
	
	reqURL := fmt.Sprintf("%s/models?%s", h.baseURL, params.Encode())
	
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}
	
	var models []HuggingFaceModel
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, err
	}
	
	return h.filterCompatibleModels(models), nil
}

// GetRecentModels gets recently updated models suitable for local use
func (h *HuggingFaceClient) GetRecentModels(ctx context.Context) ([]HuggingFaceModel, error) {
	params := url.Values{}
	params.Set("sort", "lastModified")
	params.Set("direction", "-1")
	params.Set("limit", "30")
	
	// Search for GGUF models (most compatible)
	ggufURL := fmt.Sprintf("%s/models?%s&search=gguf", h.baseURL, params.Encode())
	
	req, err := http.NewRequestWithContext(ctx, "GET", ggufURL, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}
	
	var ggufModels []HuggingFaceModel
	if err := json.NewDecoder(resp.Body).Decode(&ggufModels); err != nil {
		return nil, err
	}
	
	// Also search for MLX models if on Apple Silicon
	if IsAppleSilicon() {
		mlxURL := fmt.Sprintf("%s/models?%s&search=mlx", h.baseURL, params.Encode())
		
		req, err := http.NewRequestWithContext(ctx, "GET", mlxURL, nil)
		if err == nil {
			resp, err := h.client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				
				var mlxModels []HuggingFaceModel
				if json.NewDecoder(resp.Body).Decode(&mlxModels) == nil {
					ggufModels = append(ggufModels, mlxModels...)
				}
			}
		}
	}
	
	return h.filterCompatibleModels(ggufModels), nil
}

// GetModelFiles gets the list of files in a model repository
func (h *HuggingFaceClient) GetModelFiles(ctx context.Context, modelID string) ([]HuggingFaceFile, error) {
	reqURL := fmt.Sprintf("%s/models/%s/tree/main", h.baseURL, modelID)
	
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}
	
	var files []HuggingFaceFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}
	
	return files, nil
}

// ConvertToModelOption converts a HuggingFace model to our ModelOption format
func (h *HuggingFaceClient) ConvertToModelOption(hfModel HuggingFaceModel, selectedFile string, fileSize int64) ModelOption {
	// Determine provider based on file extension and tags
	provider := "llamacpp"
	if strings.Contains(selectedFile, ".safetensors") || containsTag(hfModel.Tags, "mlx") {
		provider = "mlx"
	}
	
	// Estimate memory requirement (rough estimate: 1.5x file size)
	memoryRequired := int64(float64(fileSize) * 1.5)
	
	// Determine tier based on size
	var tier ModelTier
	switch {
	case fileSize < 5*1024*1024*1024: // < 5GB
		tier = TierLight
	case fileSize < 15*1024*1024*1024: // < 15GB
		tier = TierBalanced
	default:
		tier = TierPowerUser
	}
	
	// Build download URL
	downloadURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", hfModel.ID, selectedFile)
	
	return ModelOption{
		ID:          strings.ReplaceAll(hfModel.ID, "/", "-") + "-" + strings.TrimSuffix(selectedFile, filepath.Ext(selectedFile)),
		Name:        fmt.Sprintf("%s (%s)", hfModel.ID, humanizeSize(fileSize)),
		Description: fmt.Sprintf("Downloads: %d | Likes: %d | Updated: %s", hfModel.Downloads, hfModel.Likes, hfModel.LastModified.Format("2006-01-02")),
		Size:        fileSize,
		Memory:      memoryRequired,
		URL:         downloadURL,
		Provider:    provider,
		Tier:        tier,
		Available:   provider != "mlx" || IsAppleSilicon(),
	}
}

// filterCompatibleModels filters models to only include ones we can run
func (h *HuggingFaceClient) filterCompatibleModels(models []HuggingFaceModel) []HuggingFaceModel {
	var filtered []HuggingFaceModel
	
	for _, model := range models {
		// Skip private, gated, or disabled models
		if model.Private || model.Gated || model.Disabled {
			continue
		}
		
		// Look for compatible tags
		hasCompatibleTag := false
		for _, tag := range model.Tags {
			tag = strings.ToLower(tag)
			if strings.Contains(tag, "gguf") || 
			   strings.Contains(tag, "ggml") ||
			   (IsAppleSilicon() && strings.Contains(tag, "mlx")) ||
			   strings.Contains(tag, "quantized") {
				hasCompatibleTag = true
				break
			}
		}
		
		// Also check model ID for GGUF/MLX indicators
		modelIDLower := strings.ToLower(model.ID)
		if !hasCompatibleTag {
			if strings.Contains(modelIDLower, "gguf") ||
			   strings.Contains(modelIDLower, "ggml") ||
			   (IsAppleSilicon() && strings.Contains(modelIDLower, "mlx")) {
				hasCompatibleTag = true
			}
		}
		
		if hasCompatibleTag {
			filtered = append(filtered, model)
		}
	}
	
	return filtered
}

// containsTag checks if a tag exists in the tags list
func containsTag(tags []string, target string) bool {
	for _, tag := range tags {
		if strings.EqualFold(tag, target) {
			return true
		}
	}
	return false
}

// humanizeSize converts bytes to human-readable format
func humanizeSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}