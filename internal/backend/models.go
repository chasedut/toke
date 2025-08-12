package backend

import (
	"fmt"
	"runtime"
)

// ModelTier represents the performance tier of a model
type ModelTier int

const (
	TierLight ModelTier = iota
	TierBalanced
	TierPowerUser
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
			Description: "Highest quality GLM. Full 8-bit precision. MLX for Apple Silicon.",
			Size:        34 * 1024 * 1024 * 1024,  // ~34 GB
			Memory:      48 * 1024 * 1024 * 1024,  // 48 GB RAM
			URL:         "https://huggingface.co/mlx-community/GLM-4.5-Air-8bit",
			Checksum:    "placeholder",
			Provider:    "mlx",
			Tier:        TierPowerUser,
			Recommended: true, // Best quality for power users
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
			Size:        17 * 1024 * 1024 * 1024,  // ~17 GB
			Memory:      24 * 1024 * 1024 * 1024,  // 24 GB RAM
			URL:         "https://huggingface.co/mlx-community/GLM-4.5-Air-4bit",
			Checksum:    "placeholder",
			Provider:    "mlx",
			Tier:        TierBalanced,
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
			Tier:        TierLight,
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
			Tier:        TierLight,
			Recommended: false,
			Available:   isAppleSilicon,
			WhyNotAvailable: func() string {
				if !isAppleSilicon {
					return "Requires Apple Silicon (M1/M2/M3/M4) Mac"
				}
				return ""
			}(),
		},
		
		// Tier 1: Light & Fast (2-4GB) - For quick responses and lower RAM (GGUF)
		{
			ID:          "qwen2.5-coder-7b-q4_k_m",
			Name:        "Qwen 2.5 Coder 7B (GGUF)",
			Description: "Best coding model. Fast and accurate. GGUF Q4_K_M quantization.",
			Size:        4794158596,              // 4.47 GB actual size
			Memory:      8 * 1024 * 1024 * 1024,  // 8 GB RAM
			URL:         "https://huggingface.co/Qwen/Qwen2.5-Coder-7B-Instruct-GGUF/resolve/main/qwen2.5-coder-7b-instruct-q4_k_m.gguf",
			Checksum:    "placeholder",
			Provider:    "llamacpp",
			Tier:        TierLight,
			Recommended: !isAppleSilicon, // Recommended for non-Apple Silicon
			Available:   true, // GGUF works everywhere
		},
		{
			ID:          "qwen2.5-3b-q4_k_m",
			Name:        "Qwen 2.5 3B",
			Description: "Smaller, faster model. Good for simple tasks. GGUF format.",
			Size:        2 * 1024 * 1024 * 1024, // 2 GB
			Memory:      4 * 1024 * 1024 * 1024, // 4 GB RAM
			URL:         "https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF/resolve/main/qwen2.5-3b-instruct-q4_k_m.gguf",
			Checksum:    "placeholder",
			Provider:    "llamacpp",
			Tier:        TierLight,
			Recommended: false,
		},
		
		// Tier 2: Balanced (8-12GB) - Better quality, still reasonable size
		{
			ID:          "qwen2.5-14b-q4_k_m",
			Name:        "Qwen 2.5 14B",
			Description: "Larger, more capable model. Excellent reasoning. GGUF format.",
			Size:        8 * 1024 * 1024 * 1024,  // 8 GB
			Memory:      16 * 1024 * 1024 * 1024, // 16 GB RAM
			URL:         "https://huggingface.co/Qwen/Qwen2.5-14B-Instruct-GGUF/resolve/main/qwen2.5-14b-instruct-q4_k_m.gguf",
			Checksum:    "placeholder",
			Provider:    "llamacpp",
			Tier:        TierBalanced,
			Recommended: true,
		},
		{
			ID:          "deepseek-coder-v2-lite-q4_k_m",
			Name:        "DeepSeek Coder V2 Lite",
			Description: "Specialized for code generation. 16B parameters. GGUF format.",
			Size:        9 * 1024 * 1024 * 1024,  // 9 GB
			Memory:      18 * 1024 * 1024 * 1024, // 18 GB RAM
			URL:         "https://huggingface.co/deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct-GGUF/resolve/main/deepseek-coder-v2-lite-instruct-q4_k_m.gguf",
			Checksum:    "placeholder",
			Provider:    "llamacpp",
			Tier:        TierBalanced,
			Recommended: false,
		},
		
		// Tier 3: Power User (40GB+) - Maximum capability
		{
			ID:          "glm-4.5-air-iq2_m",
			Name:        "GLM 4.5 Air IQ2_M",
			Description: "Massive 107B parameter model. Superior quality. 2-bit imatrix quantization.",
			Size:        44 * 1024 * 1024 * 1024,  // 44 GB
			Memory:      48 * 1024 * 1024 * 1024,  // 48 GB RAM
			URL:         "https://huggingface.co/unsloth/GLM-4.5-Air-GGUF/resolve/main/GLM-4.5-Air-IQ2_M.gguf",
			Checksum:    "placeholder",
			Provider:    "llamacpp",
			Tier:        TierPowerUser,
			Recommended: true,
		},
		{
			ID:          "glm-4.5-air-q2_k",
			Name:        "GLM 4.5 Air Q2_K",
			Description: "GLM 4.5 Air with standard 2-bit quantization. Slightly smaller.",
			Size:        45 * 1024 * 1024 * 1024,  // 45 GB
			Memory:      48 * 1024 * 1024 * 1024,  // 48 GB RAM
			URL:         "https://huggingface.co/unsloth/GLM-4.5-Air-GGUF/resolve/main/GLM-4.5-Air-Q2_K.gguf",
			Checksum:    "placeholder",
			Provider:    "llamacpp",
			Tier:        TierPowerUser,
			Recommended: false,
		},
	}
	
	// Set availability for all models
	for i := range models {
		if models[i].Available == false && models[i].Provider == "llamacpp" {
			models[i].Available = true // GGUF models work everywhere
		}
	}
	
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
	// Return Qwen 2.5 Coder 7B as the recommended option
	models := AvailableModels()
	for i := range models {
		if models[i].ID == "qwen2.5-coder-7b-q4_k_m" {
			return &models[i]
		}
	}
	return &models[0]
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
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetTierName returns a human-readable name for a tier
func GetTierName(tier ModelTier) string {
	switch tier {
	case TierLight:
		return "Light & Fast"
	case TierBalanced:
		return "Balanced"
	case TierPowerUser:
		return "Power User"
	default:
		return "Unknown"
	}
}

// GetTierDescription returns a description for a tier
func GetTierDescription(tier ModelTier) string {
	switch tier {
	case TierLight:
		return "2-4GB models for quick responses. Runs on 8GB+ RAM."
	case TierBalanced:
		return "8-12GB models with better quality. Needs 16GB+ RAM."
	case TierPowerUser:
		return "40GB+ models for maximum capability. Requires 64GB+ RAM."
	default:
		return ""
	}
}