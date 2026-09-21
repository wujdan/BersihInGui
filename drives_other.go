//go:build !windows

package main

// detectDriveType is a non-Windows stub used during local development.
func detectDriveType(root string) (string, bool) {
	return "fixed", false
}

// diskSpace is a non-Windows stub.
func diskSpace(root string) (int64, int64) {
	return 0, 0
}
