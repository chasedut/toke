// +build darwin

package backend

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// getMemoryInfoDarwin gets memory info on macOS
func getMemoryInfoDarwin(info *SystemInfo) error {
	// Get total memory using sysctl
	cmd := exec.Command("sysctl", "-n", "hw.memsize")
	output, err := cmd.Output()
	if err == nil {
		if total, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64); err == nil {
			info.TotalRAM = total
		}
	}
	
	// Get available memory using vm_stat
	cmd = exec.Command("vm_stat")
	output, err = cmd.Output()
	if err == nil {
		// Parse vm_stat output to get free + inactive memory
		lines := strings.Split(string(output), "\n")
		var pageSize int64 = 4096 // Default page size
		var freePages, inactivePages int64
		
		for _, line := range lines {
			if strings.Contains(line, "page size of") {
				// Extract page size
				parts := strings.Fields(line)
				if len(parts) >= 8 {
					if ps, err := strconv.ParseInt(parts[7], 10, 64); err == nil {
						pageSize = ps
					}
				}
			} else if strings.Contains(line, "Pages free:") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					if fp, err := strconv.ParseInt(strings.TrimSuffix(parts[2], "."), 10, 64); err == nil {
						freePages = fp
					}
				}
			} else if strings.Contains(line, "Pages inactive:") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					if ip, err := strconv.ParseInt(strings.TrimSuffix(parts[2], "."), 10, 64); err == nil {
						inactivePages = ip
					}
				}
			}
		}
		
		info.AvailableRAM = (freePages + inactivePages) * pageSize
	}
	
	// Fallback if we couldn't get available RAM
	if info.AvailableRAM == 0 && info.TotalRAM > 0 {
		// Estimate 50% available
		info.AvailableRAM = info.TotalRAM / 2
	}
	
	return nil
}

// getMemoryInfoLinux is not used on Darwin
func getMemoryInfoLinux(info *SystemInfo) error {
	return nil
}

// getMemoryInfoWindows is not used on Darwin
func getMemoryInfoWindows(info *SystemInfo) error {
	return nil
}

// getDiskSpace gets the free disk space for the given directory on macOS
func getDiskSpace(path string, info *SystemInfo) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		// Fallback for errors
		info.FreeDiskSpace = 50 * 1024 * 1024 * 1024 // Default to 50GB
		return nil
	}
	// Available space = block size * available blocks
	info.FreeDiskSpace = int64(stat.Bavail) * int64(stat.Bsize)
	return nil
}