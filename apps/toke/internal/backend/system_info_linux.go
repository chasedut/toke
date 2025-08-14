// +build linux

package backend

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// getMemoryInfoLinux gets memory info on Linux
func getMemoryInfoLinux(info *SystemInfo) error {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		
		switch fields[0] {
		case "MemTotal:":
			if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				info.TotalRAM = val * 1024 // Convert from KB to bytes
			}
		case "MemAvailable:":
			if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				info.AvailableRAM = val * 1024 // Convert from KB to bytes
			}
		}
	}
	
	// Fallback if MemAvailable is not present (older kernels)
	if info.AvailableRAM == 0 && info.TotalRAM > 0 {
		// Try to calculate from MemFree + Buffers + Cached
		file.Seek(0, 0)
		scanner = bufio.NewScanner(file)
		var memFree, buffers, cached int64
		
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			
			switch fields[0] {
			case "MemFree:":
				if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					memFree = val
				}
			case "Buffers:":
				if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					buffers = val
				}
			case "Cached:":
				if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					cached = val
				}
			}
		}
		
		info.AvailableRAM = (memFree + buffers + cached) * 1024
	}
	
	return scanner.Err()
}

// getMemoryInfoDarwin is not used on Linux
func getMemoryInfoDarwin(info *SystemInfo) error {
	return nil
}

// getMemoryInfoWindows is not used on Linux
func getMemoryInfoWindows(info *SystemInfo) error {
	return nil
}

// getDiskSpace gets the free disk space for the given directory on Linux
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