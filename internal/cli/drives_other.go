//go:build !windows

package cli

// isRemovableRoot is not needed on non-Windows platforms; external mount
// filtering relies on explicit mount enumeration instead.
func isRemovableRoot(root string) bool { return false }