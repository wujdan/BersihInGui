package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LocalDrives returns usable scan roots for the current platform.
// External & network drives are excluded by default (document tahap 2).
func LocalDrives(excludeExternal bool) []string {
	switch runtime.GOOS {
	case "windows":
		var drives []string
		for _, l := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J",
			"K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z"} {
			root := l + ":\\"
			if info, err := os.Stat(root); err == nil && info.IsDir() {
				if excludeExternal {
					// Basic heuristic: skip removable/external drives by
					// checking volume type via GetDriveType through the
					// filesystem root marker.
					if isRemovableRoot(root) {
						continue
					}
				}
				drives = append(drives, root)
			}
		}
		return drives
	case "linux":
		roots := []string{"/"}
		for _, m := range []string{"/mnt", "/media"} {
			if entries, err := os.ReadDir(m); err == nil {
				for _, e := range entries {
					p := filepath.Join(m, e.Name())
					if info, err := os.Stat(p); err == nil && info.IsDir() {
						roots = append(roots, p)
					}
				}
			}
		}
		return roots
	case "darwin":
		roots := []string{"/"}
		if entries, err := os.ReadDir("/Volumes"); err == nil {
			for _, e := range entries {
				if e.Name() == "Macintosh HD" || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				p := filepath.Join("/Volumes", e.Name())
				if info, err := os.Stat(p); err == nil && info.IsDir() {
					roots = append(roots, p)
				}
			}
		}
		return roots
	default:
		return []string{"/"}
	}
}