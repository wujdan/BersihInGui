//go:build windows

package main

import (
	"strings"

	"golang.org/x/sys/windows"
)

// detectDriveType returns a human readable drive type and whether it is
// removable/external (which we skip by default).
func detectDriveType(root string) (string, bool) {
	pathp, err := windows.UTF16PtrFromString(strings.TrimRight(root, "\\") + "\\")
	if err != nil {
		return "unknown", false
	}
	t := windows.GetDriveType(pathp)
	switch t {
	case windows.DRIVE_REMOVABLE:
		return "removable", true
	case windows.DRIVE_REMOTE:
		return "network", true
	case windows.DRIVE_CDROM:
		return "cdrom", true
	case windows.DRIVE_RAMDISK:
		return "ramdisk", true
	case windows.DRIVE_FIXED:
		return "fixed", false
	default:
		return "unknown", false
	}
}

// diskSpace returns (freeBytes, totalBytes) for a drive root on Windows.
func diskSpace(root string) (int64, int64) {
	var freeAvail, totalBytes, totalFree uint64
	pathp, err := windows.UTF16PtrFromString(strings.TrimRight(root, "\\") + "\\")
	if err != nil {
		return 0, 0
	}
	err = windows.GetDiskFreeSpaceEx(pathp, &freeAvail, &totalBytes, &totalFree)
	if err != nil {
		return 0, 0
	}
	return int64(freeAvail), int64(totalBytes)
}
