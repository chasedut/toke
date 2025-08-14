package backend

import (
	"fmt"
	"runtime"
)

// ModelTier represents the performance tier of a model
type ModelTier int

const (
	TierBottomShelf ModelTier = iota
	TierMidShelf
	TierTopShelf
)

// ModelOption represents a model that can be downloaded and run locally
type ModelOption struct {
	ID          string
	Name        string
	Description string
	Size        int64     // Download size in bytes
	Memory      int64     // Required RAM in bytes
	URL         string    // Download URL
	Checksum    string    // SHA256 checksum
	Provider    string    // Provider type (mlx, llamacpp, onnx)
	Tier        ModelTier // Performance tier
	Recommended bool      // Is this the recommended model for the tier
	Available   bool      // Is this model available on the current platform
	WhyNotAvailable string // Reason why model is not available
}

// IsAppleSilicon checks if the current system is Apple Silicon
func IsAppleSilicon() bool {
	return runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
}

// AvailableModels returns a list of models that can be run locally
func AvailableModels() []ModelOption {
	isAppleSilicon := IsAppleSilicon()
	
	models := []ModelOption{
		// MLX Models - Optimized for Apple Silicon
		{
			ID:          "glm-4.5-air-8bit",
			Name:        "GLM 4.5 Air 8-bit (MLX)",
			Description: "Highest quality 107B GLM. Full 8-bit precision. MLX for Apple Silicon.",
			Size:        110 * 1024 * 1024 * 1024,  // ~110 GB (actual: 109.61 GB)
			Memory:      128 * 1024 * 1024 * 1024,  // 128 GB RAM recommended for 8-bit
			URL:         "https://huggingface.co/lmstudio-community/GLM-4.5-Air-MLX-8bit",
			Checksum:    "placeholder",
			Provider:    "mlx",
			Tier:        TierTopShelf,
			Recommended: true, // Best quality for power users with high RAM
			Available:   isAppleSilicon,
			WhyNotAvailable: func() string {
				if !isAppleSilicon {
					return "Requires Apple Silicon (M1/M2/M3/M4) Mac"
				}
				return ""
			}(),
		},
		{
			ID:          "glm-4.5-air-4bit",
			Name:        "GLM 4.5 Air 4-bit (MLX)",
			Description: "Cutting-edge 106B model. Great balance. MLX 4-bit for Apple Silicon.",
			Size:        56 * 1024 * 1024 * 1024,  // ~56 GB (actual size with all shards)
			Memory:      24 * 1024 * 1024 * 1024,  // 24 GB RAM
			URL:         "https://huggingface.co/mlx-community/GLM-4.5-Air-4bit",
			Checksum:    "placeholder",
			Provider:    "mlx",
			Tier:        TierMidShelf,
			Recommended: true, // Recommended for Apple Silicon users with moderate RAM
			Available:   isAppleSilicon,
			WhyNotAvailable: func() string {
				if !isAppleSilicon {
					return "Requires Apple Silicon (M1/M2/M3/M4) Mac"
				}
				return ""
			}(),
		},
		{
			ID:          "glm-4.5-air-3bit",
			Name:        "GLM 4.5 Air 3-bit (MLX)",
			Description: "Same GLM model, smaller size. MLX 3-bit quantization.",
			Size:        13 * 1024 * 1024 * 1024,  // ~13 GB
			Memory:      16 * 1024 * 1024 * 1024,  // 16 GB RAM
			URL:         "https://huggingface.co/mlx-community/GLM-4.5-Air-3bit",
			Checksum:    "placeholder",
			Provider:    "mlx",
			Tier:        TierBottomShelf,
			Recommended: false,
			Available:   isAppleSilicon,
			WhyNotAvailable: func() string {
				if !isAppleSilicon {
					return "Requires Apple Silicon (M1/M2/M3/M4) Mac"
				}
				return ""
			}(),
		},
		{
			ID:          "qwen2.5-coder-7b-4bit",
			Name:        "Qwen 2.5 Coder 7B 4-bit (MLX)",
			Description: "Excellent coding model. MLX 4-bit quantization.",
			Size:        5 * 1024 * 1024 * 1024,   // ~5 GB
			Memory:      8 * 1024 * 1024 * 1024,   // 8 GB RAM
			URL:         "https://huggingface.co/mlx-community/Qwen2.5-Coder-7B-Instruct-4bit",
			Checksum:    "placeholder",
			Provider:    "mlx",
			Tier:        TierBottomShelf,
			Recommended: false,
			Available:   isAppleSilicon,
			WhyNotAvailable: func() string {
				if !isAppleSilicon {
					return "Requires Apple Silicon (M1/M2/M3/M4) Mac"
				}
				return ""
			}(),
		},
		
		// Note: GGUF/llamacpp models removed - only MLX models supported
	}
	
	// Only MLX models are supported now
	
	return models
}

// GetModelByID returns a model option by its ID
func GetModelByID(id string) *ModelOption {
	for _, model := range AvailableModels() {
		if model.ID == id {
			return &model
		}
	}
	return nil
}

// GetRecommendedModel returns the recommended model for most users
func GetRecommendedModel() *ModelOption {
	// Return GLM 4.5 Air 4-bit as the recommended option for Apple Silicon
	models := AvailableModels()
	for i := range models {
		if models[i].ID == "glm-4.5-air-4bit" && models[i].Available {
			return &models[i]
		}
	}
	// Fallback to first available model
	for i := range models {
		if models[i].Available {
			return &models[i]
		}
	}
	return nil
}

// GetModelsByTier returns all models in a specific tier
func GetModelsByTier(tier ModelTier) []ModelOption {
	var result []ModelOption
	for _, model := range AvailableModels() {
		if model.Tier == tier {
			result = append(result, model)
		}
	}
	return result
}

// GetRecommendedByTier returns the recommended model for a specific tier
func GetRecommendedByTier(tier ModelTier) *ModelOption {
	models := GetModelsByTier(tier)
	for i := range models {
		if models[i].Recommended {
			return &models[i]
		}
	}
	if len(models) > 0 {
		return &models[0]
	}
	return nil
}

// FormatSize formats bytes as human-readable string
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	// Use 2 decimal places for MB to show more granular progress
	// Use 1 decimal place for GB and above
	if exp == 1 { // MB
		return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetTierName returns a human-readable name for a tier
func GetTierName(tier ModelTier) string {
	switch tier {
	case TierBottomShelf:
		return "Light & Fast"
	case TierMidShelf:
		return "Balanced"
	case TierTopShelf:
		return "Power User"
	default:
		return "Unknown"
	}
}

// GetTierDescription returns a description for a tier
func GetTierDescription(tier ModelTier) string {
	switch tier {
	case TierBottomShelf:
		return "2-4GB models for quick responses. Runs on 8GB+ RAM."
	case TierMidShelf:
		return "8-12GB models with better quality. Needs 16GB+ RAM."
	case TierTopShelf:
		return "40GB+ models for maximum capability. Requires 64GB+ RAM."
	default:
		return ""
	}
}