//go:build windows

package cli

import (
	"strings"

	"golang.org/x/sys/windows"
)

// isRemovableRoot reports whether a drive is removable/external.
func isRemovableRoot(root string) bool {
	pathp, err := windows.UTF16PtrFromString(strings.TrimRight(root, "\\") + "\\")
	if err != nil {
		return false
	}
	t := windows.GetDriveType(pathp)
	switch t {
	case windows.DRIVE_REMOVABLE,
		windows.DRIVE_REMOTE,
		windows.DRIVE_CDROM,
		windows.DRIVE_RAMDISK:
		return true
	}
	return false
}