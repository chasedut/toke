// +build windows

package backend

import (
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	globalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	getDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// getMemoryInfoWindows gets memory info on Windows
func getMemoryInfoWindows(info *SystemInfo) error {
	var memStatus memoryStatusEx
	memStatus.Length = uint32(unsafe.Sizeof(memStatus))
	
	ret, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if ret != 0 {
		info.TotalRAM = int64(memStatus.TotalPhys)
		info.AvailableRAM = int64(memStatus.AvailPhys)
	} else {
		// Fallback values
		info.TotalRAM = 8 * 1024 * 1024 * 1024 // 8GB default
		info.AvailableRAM = 4 * 1024 * 1024 * 1024 // 4GB default
	}
	
	return nil
}

// getMemoryInfoDarwin is not used on Windows
func getMemoryInfoDarwin(info *SystemInfo) error {
	return nil
}

// getMemoryInfoLinux is not used on Windows
func getMemoryInfoLinux(info *SystemInfo) error {
	return nil
}

// getDiskSpace gets the free disk space for the given directory on Windows
func getDiskSpace(path string, info *SystemInfo) error {
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		info.FreeDiskSpace = 50 * 1024 * 1024 * 1024 // Default to 50GB
		return nil
	}
	
	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	
	if ret != 0 {
		info.FreeDiskSpace = int64(freeBytesAvailable)
	} else {
		info.FreeDiskSpace = 50 * 1024 * 1024 * 1024 // Default to 50GB
	}
	
	return nil
}