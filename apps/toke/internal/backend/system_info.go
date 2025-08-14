package backend

import (
	"fmt"
	"runtime"
)

// SystemInfo contains information about the system's hardware capabilities
type SystemInfo struct {
	TotalRAM      int64   // Total RAM in bytes
	AvailableRAM  int64   // Available RAM in bytes
	CPUCores      int     // Number of CPU cores
	IsAppleSilicon bool   // Whether running on Apple Silicon
	HasNvidiaGPU  bool    // Whether an NVIDIA GPU is available
	HasAMDGPU     bool    // Whether an AMD GPU is available
	FreeDiskSpace int64   // Free disk space in bytes in the data directory
}

// GetSystemInfo returns information about the system's hardware
func GetSystemInfo(dataDir string) (*SystemInfo, error) {
	info := &SystemInfo{
		CPUCores:       runtime.NumCPU(),
		IsAppleSilicon: IsAppleSilicon(),
	}
	
	// Get memory info
	if err := getMemoryInfo(info); err != nil {
		return nil, err
	}
	
	// Get disk space for the data directory
	if err := getDiskSpace(dataDir, info); err != nil {
		return nil, err
	}
	
	// Check for GPU availability
	checkGPUAvailability(info)
	
	return info, nil
}

// getMemoryInfo populates memory information in SystemInfo
func getMemoryInfo(info *SystemInfo) error {
	// Platform-specific memory detection
	switch runtime.GOOS {
	case "darwin":
		return getMemoryInfoDarwin(info)
	case "linux":
		return getMemoryInfoLinux(info)
	case "windows":
		return getMemoryInfoWindows(info)
	default:
		// Fallback: estimate based on runtime memory stats
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		// This is not accurate but better than nothing
		info.TotalRAM = int64(m.Sys)
		info.AvailableRAM = int64(m.Sys - m.Alloc)
		return nil
	}
}

// getDiskSpace is defined in platform-specific files

// checkGPUAvailability checks for available GPUs
func checkGPUAvailability(info *SystemInfo) {
	// For now, we'll use simple heuristics
	// TODO: Implement actual GPU detection
	
	// On macOS with Apple Silicon, we have Metal GPU support
	if runtime.GOOS == "darwin" && IsAppleSilicon() {
		// Apple Silicon has integrated GPU
		return
	}
	
	// TODO: Check for NVIDIA/AMD GPUs on other platforms
	// This would require checking for CUDA/ROCm libraries or parsing system info
}

// RecommendModelsForSystem returns a list of recommended models based on system specs
func RecommendModelsForSystem(info *SystemInfo) []*ModelOption {
	var recommendations []*ModelOption
	
	// Convert to GB for easier comparison
	totalRAMGB := info.TotalRAM / (1024 * 1024 * 1024)
	// availableRAMGB := info.AvailableRAM / (1024 * 1024 * 1024)  // Reserved for future use
	freeDiskGB := info.FreeDiskSpace / (1024 * 1024 * 1024)
	
	// For Apple Silicon, prefer MLX models
	if info.IsAppleSilicon {
		// Check RAM for MLX models
		if totalRAMGB >= 48 && freeDiskGB >= 40 {
			// Can run the 8-bit GLM model
			if model := GetModelByID("glm-4.5-air-8bit"); model != nil {
				recommendations = append(recommendations, model)
			}
		}
		if totalRAMGB >= 24 && freeDiskGB >= 60 {
			// Can run the 4-bit GLM model
			if model := GetModelByID("glm-4.5-air-4bit"); model != nil {
				recommendations = append(recommendations, model)
			}
		}
		if totalRAMGB >= 16 && freeDiskGB >= 15 {
			// Can run the 3-bit GLM model
			if model := GetModelByID("glm-4.5-air-3bit"); model != nil {
				recommendations = append(recommendations, model)
			}
		}
		if totalRAMGB >= 8 && freeDiskGB >= 6 {
			// Can run Qwen MLX
			if model := GetModelByID("qwen2.5-coder-7b-4bit"); model != nil {
				recommendations = append(recommendations, model)
			}
		}
	}
	
	// For all systems, check GGUF models
	if totalRAMGB >= 64 && freeDiskGB >= 50 {
		// Can run the largest GLM model
		if model := GetModelByID("glm-4.5-air-q2_k"); model != nil {
			recommendations = append(recommendations, model)
		}
	}
	if totalRAMGB >= 16 && freeDiskGB >= 10 {
		// Can run Qwen 14B
		if model := GetRecommendedByTier(TierMidShelf); model != nil {
			recommendations = append(recommendations, model)
		}
	}
	if totalRAMGB >= 8 && freeDiskGB >= 5 {
		// Can run Qwen 7B - the default recommended
		if model := GetRecommendedModel(); model != nil {
			recommendations = append(recommendations, model)
		}
	}
	if totalRAMGB >= 4 && freeDiskGB >= 3 {
		// Can run Qwen 3B
		if model := GetModelByID("qwen2.5-3b-q4_k_m"); model != nil {
			recommendations = append(recommendations, model)
		}
	}
	
	// If no recommendations yet, suggest the smallest model
	if len(recommendations) == 0 {
		if model := GetModelByID("qwen2.5-3b-q4_k_m"); model != nil {
			recommendations = append(recommendations, model)
		}
	}
	
	// Limit to top 3 recommendations
	if len(recommendations) > 3 {
		recommendations = recommendations[:3]
	}
	
	return recommendations
}

// FormatSystemRequirement formats a model's requirements vs available resources
func FormatSystemRequirement(model *ModelOption, info *SystemInfo) string {
	totalRAMGB := info.TotalRAM / (1024 * 1024 * 1024)
	freeDiskGB := info.FreeDiskSpace / (1024 * 1024 * 1024)
	modelRAMGB := model.Memory / (1024 * 1024 * 1024)
	modelDiskGB := model.Size / (1024 * 1024 * 1024)
	
	ramStatus := "✓"
	if modelRAMGB > totalRAMGB {
		ramStatus = "✗"
	}
	
	diskStatus := "✓"
	if modelDiskGB > freeDiskGB {
		diskStatus = "✗"
	}
	
	return fmt.Sprintf("RAM: %s %dGB/%dGB | Disk: %s %dGB/%dGB",
		ramStatus, modelRAMGB, totalRAMGB,
		diskStatus, modelDiskGB, freeDiskGB)
}